package sitepublish

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// markdown renders GitHub-flavoured Markdown (tables, strikethrough, task
// lists, autolinks). Raw HTML in a draft is dropped, not passed through, so
// an article cannot carry a script onto the customer's site.
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// imageURLs lists the distinct image addresses in a Markdown document, in
// the order they appear.
func imageURLs(source string) []string {
	var urls []string
	seen := map[string]bool{}
	walkImages(markdown.Parser().Parse(text.NewReader([]byte(source))), func(image *ast.Image) {
		destination := string(image.Destination)
		if destination != "" && !seen[destination] {
			seen[destination] = true
			urls = append(urls, destination)
		}
	})
	return urls
}

// renderHTML renders Markdown to HTML, pointing each image whose address is
// in replace at its new address.
func renderHTML(source string, replace map[string]string) (string, error) {
	document := markdown.Parser().Parse(text.NewReader([]byte(source)))
	walkImages(document, func(image *ast.Image) {
		if next, ok := replace[string(image.Destination)]; ok {
			image.Destination = []byte(next)
		}
	})
	var out bytes.Buffer
	if err := markdown.Renderer().Render(&out, []byte(source), document); err != nil {
		return "", fmt.Errorf("render article: %w", err)
	}
	return out.String(), nil
}

func walkImages(document ast.Node, visit func(*ast.Image)) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if image, ok := node.(*ast.Image); ok && entering {
			visit(image)
		}
		return ast.WalkContinue, nil
	})
}
