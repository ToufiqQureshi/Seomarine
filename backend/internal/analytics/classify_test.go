package analytics

import (
	"net/url"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		page     string
		referrer string
		domain   string
		want     source
	}{
		// AI assistants by referrer.
		{name: "chatgpt", page: "https://acme.com/", referrer: "https://chatgpt.com/", want: source{channelAI, aiChatGPT, "chatgpt.com"}},
		{name: "legacy chatgpt host", page: "https://acme.com/", referrer: "https://chat.openai.com/c/123", want: source{channelAI, aiChatGPT, "chat.openai.com"}},
		{name: "perplexity with www", page: "https://acme.com/", referrer: "https://www.perplexity.ai/search?q=x", want: source{channelAI, aiPerplexity, "perplexity.ai"}},
		{name: "gemini is not google search", page: "https://acme.com/", referrer: "https://gemini.google.com/app", want: source{channelAI, aiGemini, "gemini.google.com"}},
		{name: "bard", page: "https://acme.com/", referrer: "https://bard.google.com/", want: source{channelAI, aiGemini, "bard.google.com"}},
		{name: "claude", page: "https://acme.com/", referrer: "https://claude.ai/chat/abc", want: source{channelAI, aiClaude, "claude.ai"}},
		{name: "copilot", page: "https://acme.com/", referrer: "https://copilot.microsoft.com/", want: source{channelAI, aiCopilot, "copilot.microsoft.com"}},
		{name: "other assistant", page: "https://acme.com/", referrer: "https://chat.deepseek.com/", want: source{channelAI, aiOther, "chat.deepseek.com"}},
		{name: "uppercase referrer host", page: "https://acme.com/", referrer: "HTTPS://ChatGPT.COM/", want: source{channelAI, aiChatGPT, "chatgpt.com"}},
		{name: "referrer host with trailing dot", page: "https://acme.com/", referrer: "https://claude.ai./", want: source{channelAI, aiClaude, "claude.ai"}},
		{name: "lookalike host is not the assistant", page: "https://acme.com/", referrer: "https://notchatgpt.com/", want: source{channelReferral, "", "notchatgpt.com"}},
		{name: "assistant name as a subdomain elsewhere", page: "https://acme.com/", referrer: "https://claude.ai.evil.com/", want: source{channelReferral, "", "claude.ai.evil.com"}},

		// AI recovered from utm_source when the referrer is missing.
		{name: "chatgpt utm without referrer", page: "https://acme.com/p?utm_source=chatgpt.com", want: source{channelAI, aiChatGPT, ""}},
		{name: "bare utm tag in any case", page: "https://acme.com/p?utm_source=Perplexity", want: source{channelAI, aiPerplexity, ""}},
		{name: "utm beats a search referrer", page: "https://acme.com/?utm_source=chatgpt.com", referrer: "https://www.bing.com/", want: source{channelAI, aiChatGPT, "bing.com"}},
		{name: "unknown utm without referrer is direct", page: "https://acme.com/?utm_source=newsletter", want: source{channelDirect, "", ""}},

		// Search and social.
		{name: "google", page: "https://acme.com/", referrer: "https://www.google.com/", want: source{channelSearch, "", "google.com"}},
		{name: "google country domain", page: "https://acme.com/", referrer: "https://www.google.co.in/", want: source{channelSearch, "", "google.co.in"}},
		{name: "bing", page: "https://acme.com/", referrer: "https://www.bing.com/search?q=acme", want: source{channelSearch, "", "bing.com"}},
		{name: "duckduckgo", page: "https://acme.com/", referrer: "https://duckduckgo.com/", want: source{channelSearch, "", "duckduckgo.com"}},
		{name: "yahoo search subdomain", page: "https://acme.com/", referrer: "https://in.search.yahoo.com/", want: source{channelSearch, "", "in.search.yahoo.com"}},
		{name: "brave search", page: "https://acme.com/", referrer: "https://search.brave.com/", want: source{channelSearch, "", "search.brave.com"}},
		{name: "google lookalike label", page: "https://acme.com/", referrer: "https://googleusercontent.example/", want: source{channelReferral, "", "googleusercontent.example"}},
		{name: "facebook link shim", page: "https://acme.com/", referrer: "https://l.facebook.com/", want: source{channelSocial, "", "l.facebook.com"}},
		{name: "twitter short link", page: "https://acme.com/", referrer: "https://t.co/abc", want: source{channelSocial, "", "t.co"}},
		{name: "linkedin", page: "https://acme.com/", referrer: "https://www.linkedin.com/feed/", want: source{channelSocial, "", "linkedin.com"}},
		{name: "hacker news", page: "https://acme.com/", referrer: "https://news.ycombinator.com/", want: source{channelSocial, "", "news.ycombinator.com"}},
		{name: "social utm without referrer", page: "https://acme.com/?utm_source=facebook", want: source{channelSocial, "", ""}},
		{name: "search utm loses to a real referrer", page: "https://acme.com/?utm_source=google", referrer: "https://blog.example.org/", want: source{channelReferral, "", "blog.example.org"}},

		// Referral, direct and internal.
		{name: "other site", page: "https://acme.com/", referrer: "https://blog.example.org/post", want: source{channelReferral, "", "blog.example.org"}},
		{name: "no referrer", page: "https://acme.com/", want: source{channelDirect, "", ""}},
		{name: "whitespace referrer", page: "https://acme.com/", referrer: "   ", want: source{channelDirect, "", ""}},
		{name: "malformed referrer", page: "https://acme.com/", referrer: "http://%zz", want: source{channelDirect, "", ""}},
		{name: "referrer without scheme", page: "https://acme.com/", referrer: "chatgpt.com", want: source{channelDirect, "", ""}},
		{name: "app referrer", page: "https://acme.com/", referrer: "android-app://com.google.android.gm/", want: source{channelDirect, "", ""}},
		{name: "self referral", page: "https://acme.com/b", referrer: "https://acme.com/a", want: source{channelInternal, "", ""}},
		{name: "self referral across www and case", page: "https://www.acme.com/b", referrer: "https://ACME.com/a", want: source{channelInternal, "", ""}},
		{name: "self referral ignores a kept utm", page: "https://acme.com/b?utm_source=chatgpt.com", referrer: "https://acme.com/a", want: source{channelInternal, "", ""}},
		{name: "project subdomain is internal", page: "https://acme.com/", referrer: "https://blog.acme.com/", domain: "acme.com", want: source{channelInternal, "", ""}},
		{name: "subdomain without a project domain is a referral", page: "https://acme.com/", referrer: "https://blog.acme.com/", want: source{channelReferral, "", "blog.acme.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := url.Parse(tt.page)
			if err != nil {
				t.Fatalf("parse page: %v", err)
			}
			if got := classify(page, tt.referrer, tt.domain); got != tt.want {
				t.Errorf("classify(%q, %q, %q) = %+v, want %+v", tt.page, tt.referrer, tt.domain, got, tt.want)
			}
		})
	}
}

func TestDeviceOf(t *testing.T) {
	const (
		iPhone  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1"
		pixel   = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36"
		galaxy  = "Mozilla/5.0 (Linux; Android 13; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
		oldIPad = "Mozilla/5.0 (iPad; CPU OS 12_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/12.1 Safari/604.1"
		mac     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15"
		windows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
	)
	tests := []struct {
		name  string
		ua    string
		width int
		want  string
	}{
		{name: "iphone", ua: iPhone, width: 390, want: deviceMobile},
		{name: "android phone in landscape", ua: pixel, width: 915, want: deviceMobile},
		{name: "android tablet", ua: galaxy, width: 1280, want: deviceTablet},
		{name: "old ipad", ua: oldIPad, width: 768, want: deviceTablet},
		{name: "ipados reports a mac", ua: mac, width: 820, want: deviceTablet},
		{name: "mac", ua: mac, width: 1440, want: deviceDesktop},
		{name: "windows without width", ua: windows, width: 0, want: deviceDesktop},
		{name: "narrow unknown agent", ua: "curl/8.0", width: 360, want: deviceMobile},
		{name: "mobile width boundary", ua: windows, width: maxMobileWidth, want: deviceMobile},
		{name: "tablet width boundary", ua: windows, width: maxMobileWidth + 1, want: deviceTablet},
		{name: "desktop width boundary", ua: windows, width: maxTabletWidth, want: deviceDesktop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deviceOf(tt.ua, tt.width); got != tt.want {
				t.Errorf("deviceOf(%q, %d) = %q, want %q", tt.ua, tt.width, got, tt.want)
			}
		})
	}
}

func TestIsBot(t *testing.T) {
	tests := []struct {
		ua   string
		want bool
	}{
		{ua: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", want: true},
		{ua: "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", want: true},
		{ua: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/126.0 Safari/537.36", want: true},
		{ua: "Mozilla/5.0 (compatible; Baiduspider/2.0)", want: true},
		{ua: "", want: true},
		{ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36", want: false},
	}
	for _, tt := range tests {
		if got := isBot(tt.ua); got != tt.want {
			t.Errorf("isBot(%q) = %v, want %v", tt.ua, got, tt.want)
		}
	}
}
