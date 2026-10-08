// Package analytics records privacy-friendly pageviews from the tracker
// script, attributes them to channels (including AI assistants), and
// summarizes them per project.
package analytics

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics/geo"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

var (
	// ErrUnknownSite means no active project tracks with the site key.
	ErrUnknownSite = errors.New("unknown site")
	// ErrForeignHost means the pageview's host is not the project's domain.
	ErrForeignHost = errors.New("page host does not match the project domain")
	// ErrRateLimited means the client sent too many events for the site.
	ErrRateLimited = errors.New("rate limited")
)

const (
	// rateLimit is the most events one IP may send one site per rateWindow.
	rateLimit  = 120
	rateWindow = time.Minute
	// saltTTL keeps a day's salt past midnight UTC so in-flight requests
	// still find it; after that it is gone for good.
	saltTTL = 48 * time.Hour
)

// Hit is one pageview as the tracker reports it, already validated for
// shape by the HTTP handler.
type Hit struct {
	SiteKey     string
	Page        *url.URL // absolute http(s) URL
	Referrer    string
	ScreenWidth int
	IP          string
	UserAgent   string
}

// Summary is a project's traffic over a date range.
type Summary struct {
	Visitors  int64           `json:"visitors"`
	Pageviews int64           `json:"pageviews"`
	Series    []DayCount      `json:"series"`
	TopPages  []PageCount     `json:"topPages"`
	Channels  []ChannelCount  `json:"channels"`
	AISources []AISourceCount `json:"aiSources"`
	Referrers []ReferrerCount `json:"referrers"`
	Devices   []DeviceCount   `json:"devices"`
}

// DayCount is one UTC day of a summary's series.
type DayCount struct {
	Date      string `json:"date"`
	Visitors  int64  `json:"visitors"`
	Pageviews int64  `json:"pageviews"`
}

// PageCount is a path's pageviews.
type PageCount struct {
	Path      string `json:"path"`
	Pageviews int64  `json:"pageviews"`
}

// ChannelCount is a channel's visitors.
type ChannelCount struct {
	Channel  string `json:"channel"`
	Visitors int64  `json:"visitors"`
}

// AISourceCount is an AI assistant's visitors.
type AISourceCount struct {
	Source   string `json:"source"`
	Visitors int64  `json:"visitors"`
}

// ReferrerCount is an external referrer host's visitors.
type ReferrerCount struct {
	Host     string `json:"host"`
	Visitors int64  `json:"visitors"`
}

// DeviceCount is a device type's visitors.
type DeviceCount struct {
	Device   string `json:"device"`
	Visitors int64  `json:"visitors"`
}

// Service is the analytics use cases.
type Service struct {
	repo  repository
	redis *redis.Client
	now   func() time.Time
	geo   *geo.Lookup
}

// NewService returns a Service that stores events in db and keeps the daily
// salt and rate-limit counters in rdb.
func NewService(db *pgxpool.Pool, rdb *redis.Client, lookup *geo.Lookup) *Service {
	return &Service{repo: repository{db: db}, redis: rdb, now: time.Now, geo: lookup}
}

// EnsureSite returns projectID's site key, creating the site on first use.
// The caller must have authorized access to projectID.
func (s *Service) EnsureSite(ctx context.Context, projectID string) (string, error) {
	key, err := s.repo.ensureSite(ctx, projectID, rand.Text())
	if err != nil {
		return "", fmt.Errorf("ensure site for project %s: %w", projectID, err)
	}
	return key, nil
}

// Collect records one pageview. It returns nil without recording anything
// for crawlers, and ErrRateLimited, ErrUnknownSite or ErrForeignHost when
// the hit is refused.
//
// Visitors are counted without cookies or stored IPs: the visitor id is
// SHA-256(daily salt, site key, IP, user agent). The salt lives only in
// Redis and expires, so a visitor cannot be followed across days and the
// hash cannot be reversed once the salt is gone. The rate-limit key uses the
// same salt, so no raw IP is stored anywhere.
func (s *Service) Collect(ctx context.Context, h Hit) error {
	if isBot(h.UserAgent) {
		return nil
	}
	now := s.now().UTC()
	salt, err := s.dailySalt(ctx, now)
	if err != nil {
		return err
	}
	if err := s.checkRate(ctx, now, hash(salt, h.SiteKey, h.IP)); err != nil {
		return err
	}

	st, err := s.repo.siteByKey(ctx, h.SiteKey)
	if err != nil {
		return err
	}
	pageHost := normalizeHost(h.Page.Hostname())
	if st.Domain != "" && !withinDomain(pageHost, st.Domain) {
		return ErrForeignHost
	}

	path := h.Page.EscapedPath()
	if path == "" {
		path = "/"
	}
	var country string
	if s.geo != nil {
		country, err = s.geo.Country(h.IP)
		if err != nil {
			return fmt.Errorf("resolve event country: %w", err)
		}
	}
	return s.repo.insertEvent(ctx, event{
		SiteID:      st.ID,
		OccurredAt:  now,
		Path:        path,
		Source:      classify(h.Page, h.Referrer, st.Domain),
		Device:      deviceOf(h.UserAgent, h.ScreenWidth),
		VisitorHash: hash(salt, h.SiteKey, h.IP, h.UserAgent),
		CountryCode: country,
	})
}

// dailySalt returns the salt for now's UTC day, creating it if this is the
// day's first event. SET NX GET makes concurrent first callers agree.
func (s *Service) dailySalt(ctx context.Context, now time.Time) (string, error) {
	key := "analytics:salt:" + now.Format(time.DateOnly)
	fresh := rand.Text()
	old, err := s.redis.SetArgs(ctx, key, fresh, redis.SetArgs{Mode: "NX", Get: true, TTL: saltTTL}).Result()
	if errors.Is(err, redis.Nil) {
		return fresh, nil
	}
	if err != nil {
		return "", fmt.Errorf("load daily salt: %w", err)
	}
	return old, nil
}

// checkRate counts an event against client's fixed one-minute window.
func (s *Service) checkRate(ctx context.Context, now time.Time, client []byte) error {
	key := fmt.Sprintf("analytics:rate:%x:%d", client, now.Unix()/int64(rateWindow.Seconds()))
	var count *redis.IntCmd
	_, err := s.redis.TxPipelined(ctx, func(p redis.Pipeliner) error {
		count = p.Incr(ctx, key)
		p.ExpireNX(ctx, key, rateWindow)
		return nil
	})
	if err != nil {
		return fmt.Errorf("count event rate: %w", err)
	}
	if count.Val() > rateLimit {
		return ErrRateLimited
	}
	return nil
}

// hash returns SHA-256 of parts separated by NUL bytes, which none of them
// can contain, so different inputs never collide by concatenation.
func hash(parts ...string) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}

// maxRangeDays is the longest summary range, a leap year.
const maxRangeDays = 366

// ErrInvalidRange means a summary range is reversed or too long.
var ErrInvalidRange = fmt.Errorf("the date range must run forward and span at most %d days", maxRangeDays)

// Summarize returns projectID's traffic from the UTC day from through the
// UTC day to, both inclusive. Visitor ids rotate daily, so visitors over a
// range sum each day's unique visitors. The caller must have authorized
// access to projectID.
func (s *Service) Summarize(ctx context.Context, projectID string, from, to time.Time) (Summary, error) {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if !end.After(start) || end.Sub(start) > maxRangeDays*24*time.Hour {
		return Summary{}, ErrInvalidRange
	}
	sum, err := s.repo.summary(ctx, projectID, start, end)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize project %s: %w", projectID, err)
	}
	return sum, nil
}

// CountryCount is a site's visitor count from one ISO country.
type CountryCount struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Visitors int64   `json:"visitors"`
	Pct      float64 `json:"pct"`
}

func (s *Service) SiteID(ctx context.Context, projectID string) (int64, error) {
	return s.repo.siteIDByProject(ctx, projectID)
}

// Countries returns one page of a site's country breakdown for a signed-in
// member. Unknown countries stay out of the list but remain in the percent
// denominator, so the percentages never imply complete geolocation coverage.
func (s *Service) Countries(ctx context.Context, siteID int64, userID string, from, to time.Time, limit, offset int) ([]CountryCount, error) {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if !end.After(start) || end.Sub(start) > maxRangeDays*24*time.Hour {
		return nil, ErrInvalidRange
	}
	rows, err := s.repo.countries(ctx, siteID, userID, start, end, limit, offset)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		region, err := language.ParseRegion(rows[i].Code)
		if err != nil {
			rows[i].Name = rows[i].Code
		} else {
			rows[i].Name = display.English.Regions().Name(region)
		}
	}
	return rows, nil
}
