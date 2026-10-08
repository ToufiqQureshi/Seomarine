package branding

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const svgDataURLPrefix = "data:image/svg+xml;base64,"

var (
	safeSVGElements = map[string]bool{
		"svg": true, "g": true, "path": true, "rect": true, "circle": true,
		"ellipse": true, "line": true, "polyline": true, "polygon": true,
	}
	safeSVGAttributes = map[string]bool{
		"viewBox": true, "width": true, "height": true, "fill": true,
		"stroke": true, "stroke-width": true, "stroke-linecap": true,
		"stroke-linejoin": true, "fill-rule": true, "clip-rule": true,
		"opacity": true, "fill-opacity": true, "stroke-opacity": true,
		"transform": true, "d": true, "cx": true, "cy": true, "r": true,
		"rx": true, "ry": true, "x": true, "y": true, "x1": true,
		"x2": true, "y1": true, "y2": true, "points": true,
		"preserveAspectRatio": true,
	}
)

// sanitizeSVGDataURL parses SVG and emits a deliberately small allowlist of
// static vector shapes and presentation attributes. Script, links, CSS,
// foreign content, entities and external resource references are rejected.
func sanitizeSVGDataURL(dataURL string) (string, error) {
	if !strings.HasPrefix(dataURL, svgDataURLPrefix) {
		return "", errors.New("not an SVG data URL")
	}
	encoded := strings.TrimPrefix(dataURL, svgDataURLPrefix)
	source, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(source) == 0 {
		return "", errors.New("invalid SVG data")
	}
	decoder := xml.NewDecoder(bytes.NewReader(source))
	decoder.Strict = true
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	depth := 0
	rootSeen := false
	rootClosed := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse SVG: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if rootClosed || !safeSVGElements[token.Name.Local] || (token.Name.Space != "" && token.Name.Space != "http://www.w3.org/2000/svg") {
				return "", errors.New("unsupported SVG element")
			}
			if depth == 0 {
				if rootSeen || token.Name.Local != "svg" {
					return "", errors.New("SVG root must be svg")
				}
				rootSeen = true
			}
			clean := xml.StartElement{Name: xml.Name{Local: token.Name.Local}}
			for _, attr := range token.Attr {
				if attr.Name.Local == "xmlns" && (attr.Value == "http://www.w3.org/2000/svg" || attr.Value == "") {
					continue
				}
				if attr.Name.Space != "" || !safeSVGAttributes[attr.Name.Local] || strings.ContainsAny(attr.Value, "<>\x00") {
					return "", errors.New("unsupported SVG attribute")
				}
				lower := strings.ToLower(attr.Value)
				if strings.Contains(lower, "url(") || strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") || strings.Contains(lower, "http:") || strings.Contains(lower, "https:") {
					return "", errors.New("external SVG reference")
				}
				clean.Attr = append(clean.Attr, xml.Attr{Name: xml.Name{Local: attr.Name.Local}, Value: attr.Value})
			}
			if err := encoder.EncodeToken(clean); err != nil {
				return "", err
			}
			depth++
		case xml.EndElement:
			if depth == 0 || token.Name.Local != "svg" && !safeSVGElements[token.Name.Local] {
				return "", errors.New("unbalanced SVG")
			}
			if err := encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: token.Name.Local}}); err != nil {
				return "", err
			}
			depth--
			if depth == 0 {
				rootClosed = true
			}
		case xml.CharData:
			if depth > 0 {
				if strings.TrimSpace(string(token)) != "" {
					return "", errors.New("SVG text is not supported")
				}
			} else if strings.TrimSpace(string(token)) != "" {
				return "", errors.New("content outside SVG root")
			}
		case xml.Comment:
			// Comments do not affect the rendered image and are omitted.
		case xml.Directive, xml.ProcInst:
			return "", errors.New("SVG directives are not supported")
		default:
			return "", errors.New("unsupported SVG content")
		}
	}
	if !rootSeen || !rootClosed || depth != 0 {
		return "", errors.New("incomplete SVG")
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	return svgDataURLPrefix + base64.StdEncoding.EncodeToString(output.Bytes()), nil
}
