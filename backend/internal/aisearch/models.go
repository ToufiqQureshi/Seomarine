package aisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

// The models of a prompt run, as the provider names their endpoints.
const (
	modelChatGPT    = "chat_gpt"
	modelClaude     = "claude"
	modelGemini     = "gemini"
	modelPerplexity = "perplexity"
)

var promptModels = []string{modelChatGPT, modelClaude, modelGemini, modelPerplexity}

const catalogTTL = time.Hour

// fallbackModelNames are the aliases DataForSEO accepted on 2026-08-25, newest
// first. They are used when the catalog is unreachable or lists no flagship.
var fallbackModelNames = map[string][]string{
	modelChatGPT:    {"gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.2", "gpt-5.1", "gpt-5"},
	modelClaude:     {"claude-sonnet-5", "claude-sonnet-4-6", "claude-sonnet-4-5"},
	modelGemini:     {"gemini-2.5-pro"},
	modelPerplexity: {"sonar-reasoning-pro", "sonar-pro", "sonar"},
}

// pinnedModelNames wins over the flagship rule while the catalog still lists
// it. ChatGPT is pinned to the 5.6 tier by the maintainer's choice: the rule
// cannot rank tier variants (terra, sol, luna).
var pinnedModelNames = map[string]string{modelChatGPT: "gpt-5.6-luna"}

// flagshipRules match the plain, undated, unsuffixed family name per model.
// Dated releases duplicate their alias, and suffixed variants are size or
// speed tiers whose default semantics we cannot know, so the highest plain
// alias is what "latest" means. The provider resolves an alias to its newest
// dated version.
var flagshipRules = map[string]*regexp.Regexp{
	modelChatGPT:    regexp.MustCompile(`^gpt-(\d+(?:\.\d+)*)$`),
	modelClaude:     regexp.MustCompile(`^claude-sonnet-(\d+(?:-\d+)*)$`),
	modelGemini:     regexp.MustCompile(`^gemini-(\d+(?:\.\d+)*)-pro$`),
	modelPerplexity: regexp.MustCompile(`^sonar-reasoning-pro$`),
}

// modelCatalog resolves which model name to ask for. The names come from
// DataForSEO's free per-provider catalog, so new model families need no code
// change, and a name is checked against it before every paid call because the
// provider bills a task that fails with Invalid Field: model_name.
type modelCatalog struct {
	client *dataforseo.Client
	logger *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]catalogEntry
}

type catalogEntry struct {
	names     []string
	fetchedAt time.Time
}

func newModelCatalog(client *dataforseo.Client, logger *slog.Logger) *modelCatalog {
	return &modelCatalog{client: client, logger: logger, now: time.Now, cache: map[string]catalogEntry{}}
}

// names lists the models the provider accepts for model. A failed or empty
// fetch falls back to the snapshot and is not cached, so an outage heals itself.
func (c *modelCatalog) names(ctx context.Context, organizationID, model string) []string {
	c.mu.Lock()
	entry, ok := c.cache[model]
	c.mu.Unlock()
	if ok && c.now().Sub(entry.fetchedAt) < catalogTTL {
		return entry.names
	}

	names, err := c.fetch(ctx, organizationID, model)
	if err != nil || len(names) == 0 {
		if err != nil {
			c.logger.WarnContext(ctx, "fetch DataForSEO model catalog", "model", model, "err", err)
		}
		return fallbackModelNames[model]
	}
	c.mu.Lock()
	c.cache[model] = catalogEntry{names: names, fetchedAt: c.now()}
	c.mu.Unlock()
	return names
}

func (c *modelCatalog) fetch(ctx context.Context, organizationID, model string) ([]string, error) {
	payload, err := c.client.Do(ctx, organizationID, http.MethodGet, "/v3/ai_optimization/"+model+"/llm_responses/models", nil, true)
	if err != nil {
		return nil, fmt.Errorf("get model catalog: %w", err)
	}
	results, err := dataforseo.Results(payload)
	if err != nil {
		return nil, fmt.Errorf("read model catalog: %w", err)
	}
	var names []string
	for _, raw := range results {
		var row struct {
			ModelName string `json:"model_name"`
		}
		if json.Unmarshal(raw, &row) == nil && row.ModelName != "" {
			names = append(names, row.ModelName)
		}
	}
	return names, nil
}

// latest returns the model name to use for model.
func (c *modelCatalog) latest(ctx context.Context, organizationID, model string) string {
	return pickLatest(model, c.names(ctx, organizationID, model))
}

// known reports whether the provider accepts modelName for model.
func (c *modelCatalog) known(ctx context.Context, organizationID, model, modelName string) bool {
	return slices.Contains(c.names(ctx, organizationID, model), modelName)
}

func pickLatest(model string, names []string) string {
	if pinned, ok := pinnedModelNames[model]; ok && slices.Contains(names, pinned) {
		return pinned
	}
	var best string
	var bestVersion []int
	for _, name := range names {
		match := flagshipRules[model].FindStringSubmatch(name)
		if match == nil {
			continue
		}
		version := []int{0} // the Perplexity rule has no version to compare
		if len(match) > 1 {
			version = parseVersion(match[1])
		}
		if best == "" || slices.Compare(padTo(version, len(bestVersion)), padTo(bestVersion, len(version))) > 0 {
			best, bestVersion = name, version
		}
	}
	if best != "" {
		return best
	}
	// No flagship alias: take the newest snapshot name the catalog still lists,
	// so the pick passes the catalog check before a paid call.
	fallback := fallbackModelNames[model]
	for _, name := range fallback {
		if slices.Contains(names, name) {
			return name
		}
	}
	return fallback[0]
}

// parseVersion reads "5.5" or "4-6" as [5 5] or [4 6].
func parseVersion(s string) []int {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' })
	version := make([]int, len(parts))
	for i, part := range parts {
		version[i], _ = strconv.Atoi(part)
	}
	return version
}

// padTo extends v with zeros to n parts, so 5.5 equals 5.5.0.
func padTo(v []int, n int) []int {
	if len(v) >= n {
		return v
	}
	return append(slices.Clone(v), make([]int, n-len(v))...)
}
