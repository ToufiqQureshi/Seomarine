// Package branding stores white-label branding for an organization's
// client-facing reports.
package branding

// MaxLogoChars is roughly 150 KB of image once base64 is decoded: a logo, not
// a photo. The report sandbox CSP allows `img-src data:` only, so the logo
// travels as a data URL rather than an R2 object or a remote URL.
const MaxLogoChars = 200_000

// MaxNameChars bounds a brand name.
const MaxNameChars = 60

// MaxWebsiteChars bounds the website shown on shared reports.
const MaxWebsiteChars = 200

// DefaultAccent is the product accent, used when an organization sets none.
const DefaultAccent = "#2563eb"

// Branding is an organization's white-label branding. A missing row means the
// default product branding, so reads may yield no Branding at all.
type Branding struct {
	BrandName   string  `json:"brandName"`
	AccentColor string  `json:"accentColor"`
	LogoDataURL *string `json:"logoDataUrl"`
	WebsiteURL  *string `json:"websiteUrl"`
	UpdatedAt   string  `json:"updatedAt"`
}

// Input is a validated branding write, trimmed the way the legacy schema
// parsed it.
type Input struct {
	BrandName   string
	AccentColor string
	LogoDataURL *string
	WebsiteURL  *string
}
