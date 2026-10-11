// Package ahrefs loads free Ahrefs domain ratings and caches them in Redis.
package ahrefs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/net/idna"
)

const (
	cachePrefix = "ahrefs-dr:"
	cacheTTL    = 24 * time.Hour
	endpoint    = "https://api.ahrefs.com/v3/public/domain-rating-free"
	maxDomains  = 100
	batchSize   = 20
)

// Service fetches public Domain Rating values. A nil value means Ahrefs has no
// rating for the host or the optional lookup failed.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Service struct {
	redis  *redis.Client
	client httpDoer
}

// New creates an Ahrefs rating service backed by the shared Redis instance.
func New(rdb *redis.Client) *Service {
	return &Service{redis: rdb, client: &http.Client{Timeout: 5 * time.Second}}
}

// Ratings returns one result per original input, preserving the caller's keys.
func (s *Service) Ratings(ctx context.Context, domains []string) (map[string]*float64, error) {
	if len(domains) > maxDomains {
		return nil, fmt.Errorf("at most %d domains are allowed", maxDomains)
	}
	out := make(map[string]*float64, len(domains))
	originals := make(map[string][]string)
	for _, original := range domains {
		domain, err := normalizeDomain(original)
		if err != nil {
			return nil, err
		}
		originals[domain] = append(originals[domain], original)
	}
	type result struct {
		domain string
		rating *float64
	}
	results := make(chan result, len(originals))
	keys := make([]string, 0, len(originals))
	for domain := range originals {
		keys = append(keys, domain)
	}
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		var wg sync.WaitGroup
		for _, domain := range keys[start:end] {
			wg.Add(1)
			go func(domain string) {
				defer wg.Done()
				rating, _ := s.lookup(ctx, domain)
				results <- result{domain: domain, rating: rating}
			}(domain)
		}
		wg.Wait()
	}
	close(results)
	for item := range results {
		for _, original := range originals[item.domain] {
			out[original] = item.rating
		}
	}
	return out, nil
}

func (s *Service) lookup(ctx context.Context, domain string) (*float64, error) {
	key := cachePrefix + domain
	if s.redis != nil {
		cached, err := s.redis.Get(ctx, key).Result()
		if err == nil {
			var value *float64
			if json.Unmarshal([]byte(cached), &value) == nil {
				return value, nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?target="+url.QueryEscape(domain), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Ahrefs returned %s", response.Status)
	}
	var body struct {
		DomainRating struct {
			DomainRating float64 `json:"domain_rating"`
		} `json:"domain_rating"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	var rating *float64
	if body.DomainRating.DomainRating > 0 && body.DomainRating.DomainRating <= 100 {
		value := body.DomainRating.DomainRating
		rating = &value
	}
	if s.redis != nil {
		if bytes, err := json.Marshal(rating); err == nil {
			_ = s.redis.Set(ctx, key, bytes, cacheTTL).Err()
		}
	}
	return rating, nil
}

func normalizeDomain(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || len(input) > 253 {
		return "", fmt.Errorf("domain is invalid")
	}
	candidate := input
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("domain is invalid")
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")))
	if err != nil {
		return "", fmt.Errorf("domain is invalid")
	}
	return strings.TrimPrefix(host, "www."), nil
}
