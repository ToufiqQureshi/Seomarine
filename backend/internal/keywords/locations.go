package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

const locationCacheTTL = 30 * 24 * time.Hour

// ErrLocationCache marks a cache failure that should fail closed before origin fetch.
var ErrLocationCache = errors.New("SERP location cache unavailable")

type locationCache interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte, time.Duration) error
}

type redisLocationCache struct{ Client *redis.Client }

func (c redisLocationCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.Client.Get(ctx, key).Bytes()
}
func (c redisLocationCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.Client.Set(ctx, key, value, ttl).Err()
}

type locationFill struct {
	done chan struct{}
	rows []SerpLocation
	err  error
}

// LocationService fetches and caches canonical DataForSEO locations by country.
type LocationService struct {
	client   *dataforseo.Client
	cache    locationCache
	limiter  locationLimiter
	logger   *slog.Logger
	mu       sync.Mutex
	inflight map[string]*locationFill
}

// NewLocationService creates a service using Redis for the shared country cache.
func NewLocationService(client *dataforseo.Client, rdb *redis.Client, logger *slog.Logger) *LocationService {
	if logger == nil {
		logger = slog.Default()
	}
	return &LocationService{client: client, cache: redisLocationCache{Client: rdb}, limiter: redisLocationLimiter{Client: rdb}, logger: logger, inflight: make(map[string]*locationFill)}
}

// Search returns up to 50 matching canonical locations for a country.
func (s *LocationService) Search(ctx context.Context, organizationID, country, query string) ([]SerpLocation, error) {
	all, err := s.Locations(ctx, organizationID, country)
	if err != nil {
		return nil, err
	}
	return RankSerpLocations(query, all, country), nil
}

// LocationNames returns the canonical names of the country's cities, counties and regions.
func (s *LocationService) LocationNames(ctx context.Context, organizationID, country string) ([]string, error) {
	rows, err := s.Locations(ctx, organizationID, country)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.LocationName
	}
	return names, nil
}

// Locations returns the country's filtered registry, using a 30-day shared cache.
func (s *LocationService) Locations(ctx context.Context, organizationID, country string) ([]SerpLocation, error) {
	key := "serp-locations:" + strings.ToLower(country)
	cached, err := s.cache.Get(ctx, key)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("%w: %w", ErrLocationCache, err)
	}
	if err == nil {
		var rows []SerpLocation
		if json.Unmarshal(cached, &rows) == nil && len(rows) > 0 {
			return rows, nil
		}
	}
	return s.fill(ctx, organizationID, strings.ToLower(country), key)
}

func (s *LocationService) fill(ctx context.Context, organizationID, country, key string) ([]SerpLocation, error) {
	s.mu.Lock()
	if existing := s.inflight[country]; existing != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-existing.done:
			return existing.rows, existing.err
		}
	}
	current := &locationFill{done: make(chan struct{})}
	s.inflight[country] = current
	s.mu.Unlock()

	current.rows, current.err = s.fetch(ctx, organizationID, country)
	if current.err == nil && len(current.rows) > 0 {
		payload, err := json.Marshal(current.rows)
		if err == nil {
			writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			if err := s.cache.Set(writeCtx, key, payload, locationCacheTTL); err != nil {
				s.logger.WarnContext(writeCtx, "write SERP location cache failed", "err", err)
			}
			cancel()
		}
	}
	s.mu.Lock()
	delete(s.inflight, country)
	close(current.done)
	s.mu.Unlock()
	return current.rows, current.err
}

func (s *LocationService) fetch(ctx context.Context, organizationID, country string) ([]SerpLocation, error) {
	response, err := s.client.Do(ctx, organizationID, http.MethodGet, "/v3/serp/google/locations/"+country, nil, true)
	if err != nil {
		return nil, fmt.Errorf("fetch SERP locations: %w", err)
	}
	items, err := dataforseo.Results(response)
	if err != nil {
		return nil, fmt.Errorf("read SERP locations: %w", err)
	}
	rows := make([]SerpLocation, 0, len(items))
	for _, item := range items {
		var row struct {
			Code *int    `json:"location_code"`
			Name *string `json:"location_name"`
			Type *string `json:"location_type"`
		}
		if json.Unmarshal(item, &row) != nil || row.Code == nil || row.Name == nil || row.Type == nil || !includedLocationType(*row.Type) {
			continue
		}
		rows = append(rows, SerpLocation{LocationCode: *row.Code, LocationName: *row.Name, LocationType: *row.Type, DisplayLabel: market.FormatLocationLabel(*row.Name, 0)})
	}
	return rows, nil
}

func includedLocationType(value string) bool {
	switch value {
	case "City", "County", "Municipality", "DMA Region", "Region":
		return true
	default:
		return false
	}
}
