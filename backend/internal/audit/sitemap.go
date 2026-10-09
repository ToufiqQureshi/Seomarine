package audit

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
)

// maxSitemapBytes is the largest sitemap document the discovery reads. Sitemap
// shards can legally reach 50 MB and several are read at once, so oversized
// shards are skipped whole: truncated XML would not parse anyway.
const maxSitemapBytes = 10 * 1024 * 1024

// isProbablySitemapXML reports whether a response looks like a sitemap, by
// content type or by its leading bytes. Mirrors isProbablySitemapXml.
func isProbablySitemapXML(contentType, body string) bool {
	if strings.Contains(strings.ToLower(contentType), "xml") {
		return true
	}
	trimmed := strings.ToLower(strings.TrimLeft(body, " \t\r\n"))
	return strings.HasPrefix(trimmed, "<?xml") ||
		strings.HasPrefix(trimmed, "<urlset") ||
		strings.HasPrefix(trimmed, "<sitemapindex")
}

// sitemapSections is the parsed `<sitemap>` and `<url>` entries of a sitemap.
type sitemapSections struct {
	Sitemaps []string
	URLs     []string
}

// parseSitemapXML extracts the `<loc>` values of `<sitemap>` and `<url>`
// entries, tolerating the sitemapindex and urlset document shapes.
func parseSitemapXML(body string) sitemapSections {
	decoder := xml.NewDecoder(strings.NewReader(body))
	decoder.Strict = false
	sections := sitemapSections{}
	// stack tracks the ancestor element local names so a <loc> knows whether it
	// belongs to a <sitemap> or a <url>.
	stack := make([]string, 0, 8)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err != io.EOF {
				// Malformed XML: keep what parsed so far, like fast-xml-parser
				// returning a partial document.
			}
			break
		}
		switch value := token.(type) {
		case xml.StartElement:
			stack = append(stack, value.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 || stack[len(stack)-1] != "loc" {
				continue
			}
			if len(stack) < 2 {
				continue
			}
			loc := strings.TrimSpace(string(value))
			if loc == "" {
				continue
			}
			switch stack[len(stack)-2] {
			case "sitemap":
				sections.Sitemaps = append(sections.Sitemaps, loc)
			case "url":
				sections.URLs = append(sections.URLs, loc)
			}
		}
	}
	return sections
}

// readBodyCapped reads a response body up to maxBytes, returning ok=false when
// the body exceeds the cap.
func readBodyCapped(response *http.Response, maxBytes int) (string, bool) {
	if response == nil || response.Body == nil {
		return "", true
	}
	buffer := make([]byte, 0, 64*1024)
	chunk := make([]byte, 32*1024)
	for {
		n, err := response.Body.Read(chunk)
		if n > 0 {
			if len(buffer)+n > maxBytes {
				return "", false
			}
			buffer = append(buffer, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	return decodeUTF8Prefix(buffer), true
}
