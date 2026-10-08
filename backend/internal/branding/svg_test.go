package branding

import (
	"encoding/base64"
	"strings"
	"testing"
)

func svgURL(markup string) string {
	return svgDataURLPrefix + base64.StdEncoding.EncodeToString([]byte(markup))
}

func TestSanitizeSVGDataURL(t *testing.T) {
	got, err := sanitizeSVGDataURL(svgURL(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0" fill="#123456"/></svg>`))
	if err != nil {
		t.Fatalf("sanitize safe SVG: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, svgDataURLPrefix))
	if err != nil {
		t.Fatalf("decode sanitized SVG: %v", err)
	}
	if string(decoded) != `<svg viewBox="0 0 10 10"><path d="M0 0" fill="#123456"></path></svg>` {
		t.Fatalf("unexpected sanitized SVG: %s", decoded)
	}
}

func TestSanitizeSVGDataURLRejectsActiveOrExternalContent(t *testing.T) {
	tests := []struct {
		name   string
		markup string
	}{
		{name: "script element", markup: `<svg><script>alert(1)</script></svg>`},
		{name: "event attribute", markup: `<svg onload="alert(1)"></svg>`},
		{name: "external image", markup: `<svg><image href="https://attacker.invalid/a.svg"/></svg>`},
		{name: "style URL", markup: `<svg><path fill="url(https://attacker.invalid/a)"/></svg>`},
		{name: "doctype", markup: `<!DOCTYPE svg><svg></svg>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := sanitizeSVGDataURL(svgURL(tt.markup)); err == nil {
				t.Fatal("expected active or external SVG content to be rejected")
			}
		})
	}
}
