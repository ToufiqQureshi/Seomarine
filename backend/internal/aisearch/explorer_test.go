package aisearch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	chatGPTResponsePath = "/v3/ai_optimization/chat_gpt/llm_responses/live"
	claudeResponsePath  = "/v3/ai_optimization/claude/llm_responses/live"
	geminiResponsePath  = "/v3/ai_optimization/gemini/llm_responses/live"
)

func promptInput(models ...string) PromptExplorerInput {
	return PromptExplorerInput{Prompt: "what is the best seo tool", Models: models, WebSearch: true}
}

func firstSuccess(t *testing.T, result PromptExplorerResult, i int) ModelSuccess {
	t.Helper()
	got, ok := result.Results[i].(ModelSuccess)
	if !ok {
		t.Fatalf("result %d = %+v, want a ModelSuccess", i, result.Results[i])
	}
	return got
}

func TestExplorePromptRunsEachModelOnItsOwnEndpoint(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := promptInput(modelChatGPT, modelClaude, modelGemini)
	in.HighlightBrand = "acme"

	result, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", in)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(result.Results))
	}
	for i, model := range in.Models {
		got := firstSuccess(t, result, i)
		if got.Model != model || got.Text == "" || got.BrandMentioned == nil || !*got.BrandMentioned {
			t.Errorf("%s result = %+v, want an answer that mentions acme", model, got)
		}
		if !got.Citations[0].MatchedBrand {
			t.Errorf("%s citation = %+v, want it matched to acme", model, got.Citations[0])
		}
	}
	if *result.HighlightBrand != "acme" {
		t.Errorf("HighlightBrand = %v, want acme", result.HighlightBrand)
	}

	field := func(path, name string) any {
		var body []map[string]any
		if err := json.Unmarshal([]byte(api.requestsTo(path)[0].body), &body); err != nil {
			t.Fatal(err)
		}
		return body[0][name]
	}
	// Only Claude accepts force_web_search; the others reject the field.
	if field(claudeResponsePath, "force_web_search") != true {
		t.Error("claude request lacks force_web_search")
	}
	if field(chatGPTResponsePath, "force_web_search") != nil || field(geminiResponsePath, "force_web_search") != nil {
		t.Error("chat_gpt or gemini request carries force_web_search, which they reject")
	}
	if field(chatGPTResponsePath, "model_name") != "gpt-5.6-luna" {
		t.Errorf("chat_gpt model_name = %v, want the pinned gpt-5.6-luna", field(chatGPTResponsePath, "model_name"))
	}
	if field(chatGPTResponsePath, "max_output_tokens") != 4096.0 {
		t.Errorf("max_output_tokens = %v, want 4096", field(chatGPTResponsePath, "max_output_tokens"))
	}
}

func TestExplorePromptRetriesOnceWhenTheModelAnsweredFromMemory(t *testing.T) {
	tests := []struct {
		name       string
		replies    []taskReply
		wantCalls  int
		wantSearch bool
		wantText   string
	}{
		{
			name:       "uses the retry when it searched",
			replies:    []taskReply{okReply(answer(false, "from memory")), okReply(answer(true, "searched", "https://example.com/post"))},
			wantCalls:  2,
			wantSearch: true,
			wantText:   "searched",
		},
		{
			name:       "does not retry when the first answer searched",
			replies:    []taskReply{okReply(answer(true, "searched", "https://example.com/post"))},
			wantCalls:  1,
			wantSearch: true,
			wantText:   "searched",
		},
		{
			name:       "keeps the first answer when the retry did not search either",
			replies:    []taskReply{okReply(answer(false, "first")), okReply(answer(false, "second"))},
			wantCalls:  2,
			wantSearch: false,
			wantText:   "first",
		},
		{
			name:       "keeps the paid first answer when the retry fails",
			replies:    []taskReply{okReply(answer(false, "first")), failReply(50000, "Internal Error.")},
			wantCalls:  2,
			wantSearch: false,
			wantText:   "first",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t)
			api.on(chatGPTResponsePath, func(call int, _ string) taskReply { return tt.replies[call-1] })
			svc := newTestService(t, api)

			result, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", promptInput(modelChatGPT))
			if err != nil {
				t.Fatal(err)
			}

			got := firstSuccess(t, result, 0)
			if calls := len(api.requestsTo(chatGPTResponsePath)); calls != tt.wantCalls {
				t.Errorf("model calls = %d, want %d", calls, tt.wantCalls)
			}
			if got.WebSearch != tt.wantSearch || got.Text != tt.wantText {
				t.Errorf("answer = webSearch %v %q, want %v %q", got.WebSearch, got.Text, tt.wantSearch, tt.wantText)
			}
		})
	}
}

func TestExplorePromptDoesNotRetryWhenWebSearchIsOff(t *testing.T) {
	api := newFakeAPI(t)
	api.on(chatGPTResponsePath, func(int, string) taskReply { return okReply(answer(false, "no search wanted")) })
	svc := newTestService(t, api)
	in := promptInput(modelChatGPT)
	in.WebSearch = false

	if _, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", in); err != nil {
		t.Fatal(err)
	}
	if calls := len(api.requestsTo(chatGPTResponsePath)); calls != 1 {
		t.Errorf("model calls = %d, want 1", calls)
	}
}

func TestExplorePromptCachesAnswersAndReappliesTheBrand(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	org := newOrg()
	ctx := context.Background()

	first := promptInput(modelChatGPT)
	first.HighlightBrand = "acme"
	if _, err := svc.ExplorePrompt(ctx, org, "p", first); err != nil {
		t.Fatal(err)
	}

	second := promptInput(modelChatGPT)
	second.Prompt = "  what is the   best seo tool " // only whitespace differs
	second.HighlightBrand = "nobody"
	result, err := svc.ExplorePrompt(ctx, org, "p", second)
	if err != nil {
		t.Fatal(err)
	}

	if calls := len(api.requestsTo(chatGPTResponsePath)); calls != 1 {
		t.Errorf("model calls = %d, want 1 (second served from cache)", calls)
	}
	got := firstSuccess(t, result, 0)
	if got.BrandMentioned == nil || *got.BrandMentioned || got.Citations[0].MatchedBrand {
		t.Errorf("cached answer = %+v, want the new brand applied (not mentioned)", got)
	}

	third := promptInput(modelChatGPT)
	third.Prompt = "What is the best SEO tool" // case differs: a different prompt
	if _, err := svc.ExplorePrompt(ctx, org, "p", third); err != nil {
		t.Fatal(err)
	}
	if calls := len(api.requestsTo(chatGPTResponsePath)); calls != 2 {
		t.Errorf("model calls = %d, want 2 (case-sensitive prompt)", calls)
	}
}

func TestExplorePromptDoesNotCacheFailures(t *testing.T) {
	api := newFakeAPI(t)
	api.on(claudeResponsePath, func(int, string) taskReply { return failReply(50000, "Internal Error.") })
	svc := newTestService(t, api)
	org := newOrg()

	for range 2 {
		result, err := svc.ExplorePrompt(context.Background(), org, "p", promptInput(modelClaude))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := result.Results[0].(ModelError); !ok || got.ErrorCode != "UPSTREAM_ERROR" || got.Model != modelClaude {
			t.Fatalf("result = %+v, want an UPSTREAM_ERROR for claude", result.Results[0])
		}
	}
	if calls := len(api.requestsTo(claudeResponsePath)); calls != 2 {
		t.Errorf("model calls = %d, want 2 (a failure is not cached)", calls)
	}
}

func TestExplorePromptOneModelFailingDoesNotHideTheOthers(t *testing.T) {
	api := newFakeAPI(t)
	api.on(claudeResponsePath, func(int, string) taskReply { return failReply(40501, "Invalid Field: 'model_name'.") })
	svc := newTestService(t, api)

	result, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", promptInput(modelChatGPT, modelClaude, modelGemini))
	if err != nil {
		t.Fatal(err)
	}

	firstSuccess(t, result, 0)
	firstSuccess(t, result, 2)
	failed, ok := result.Results[1].(ModelError)
	if !ok || strings.Contains(failed.Message, "model_name") {
		t.Errorf("failed result = %+v, want a generic message that does not leak provider detail", result.Results[1])
	}
}

func TestExplorePromptRefusesACountryTheModelCannotTakeWithoutCallingIt(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := promptInput(modelChatGPT, modelGemini)
	in.WebSearchCountryCode = "BG"

	result, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", in)
	if err != nil {
		t.Fatal(err)
	}

	firstSuccess(t, result, 0)
	if got, ok := result.Results[1].(ModelError); !ok || got.ErrorCode != "UNSUPPORTED_COUNTRY" {
		t.Errorf("gemini result = %+v, want UNSUPPORTED_COUNTRY", result.Results[1])
	}
	if calls := len(api.requestsTo(geminiResponsePath)); calls != 0 {
		t.Errorf("gemini calls = %d, want 0 (a rejected task is still billed)", calls)
	}
	var body []map[string]any
	if err := json.Unmarshal([]byte(api.requestsTo(chatGPTResponsePath)[0].body), &body); err != nil {
		t.Fatal(err)
	}
	if body[0]["web_search_country_iso_code"] != "BG" {
		t.Errorf("web_search_country_iso_code = %v, want BG", body[0]["web_search_country_iso_code"])
	}
}

func TestExplorePromptIgnoresTheCountryWhenWebSearchIsOff(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := promptInput(modelGemini)
	in.WebSearch, in.WebSearchCountryCode = false, "US"

	result, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", in)
	if err != nil {
		t.Fatal(err)
	}

	got := firstSuccess(t, result, 0)
	if got.WebSearchCountryCode != nil {
		t.Errorf("WebSearchCountryCode = %v, want null when search is off", *got.WebSearchCountryCode)
	}
	if strings.Contains(api.requestsTo(geminiResponsePath)[0].body, "web_search_country_iso_code") {
		t.Error("the country was sent although web search is off; every model rejects that")
	}
}

func TestExplorePromptBillingIssueFailsTheRequest(t *testing.T) {
	api := newFakeAPI(t)
	api.on(claudeResponsePath, func(int, string) taskReply { return failReply(40200, "Payment Required.") })
	svc := newTestService(t, api)

	_, err := svc.ExplorePrompt(context.Background(), newOrg(), "p", promptInput(modelChatGPT, modelClaude))

	if !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("ExplorePrompt() error = %v, want ErrBillingIssue", err)
	}
}

func TestExplorePromptRefusesAnUnknownModelNameBeforeSpending(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	_, err := svc.provider.llmResponse(context.Background(), responseRequest{
		organizationID: "org", model: modelClaude, modelName: "claude-made-up", prompt: "hi", webSearch: true,
	})

	if !errors.Is(err, errInvalidRequest) {
		t.Fatalf("llmResponse() error = %v, want errInvalidRequest", err)
	}
	if calls := len(api.requestsTo(claudeResponsePath)); calls != 0 {
		t.Errorf("model calls = %d, want 0", calls)
	}
}

func TestLLMResponseNeverSendsACountryWithoutWebSearch(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)

	_, err := svc.provider.llmResponse(context.Background(), responseRequest{
		organizationID: "org", model: modelGemini, modelName: "gemini-2.5-pro", prompt: "hi",
		webSearch: false, countryCode: "US",
	})

	if err != nil {
		t.Fatalf("llmResponse() error = %v", err)
	}
	if body := api.requestsTo(geminiResponsePath)[0].body; strings.Contains(body, "web_search_country_iso_code") {
		t.Errorf("request %s carries a country with web search off, which every model rejects", body)
	}
}
