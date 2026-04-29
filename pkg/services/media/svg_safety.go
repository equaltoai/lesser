package media

import (
	"bytes"
	"regexp"
	"strings"
)

var svgEventAttributePattern = regexp.MustCompile(`(?i)\son[a-z0-9_-]+\s*=`)
var svgExternalReferencePattern = regexp.MustCompile(`(?i)\s(?:href|src|xlink:href)\s*=\s*['"]\s*(?:data:|file:|https?:|javascript:)`)

// ValidateSVGUpload rejects SVG uploads that contain active content or external
// references. SVG is XML but browsers execute scripts, event handlers, and CSS
// URLs in many SVG contexts, so lesser accepts only inert inline SVG markup.
func ValidateSVGUpload(contentType string, data []byte) error {
	if !isSVGContentType(contentType) {
		return nil
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ErrMediaFileDataRequired
	}

	lower := strings.ToLower(string(trimmed))
	if !strings.Contains(lower, "<svg") {
		return ErrMediaUnsafeSVG
	}

	blockedFragments := []string{
		"<!doctype",
		"<!entity",
		"<?xml-stylesheet",
		"<script",
		"<foreignobject",
		"<iframe",
		"<object",
		"<embed",
		"<link",
		"<meta",
		"<style",
		"<image",
		"javascript:",
		"data:text/html",
		"data:image/svg",
		"url(",
	}
	for _, fragment := range blockedFragments {
		if strings.Contains(lower, fragment) {
			return ErrMediaUnsafeSVG
		}
	}

	if svgEventAttributePattern.Match(trimmed) || svgExternalReferencePattern.Match(trimmed) {
		return ErrMediaUnsafeSVG
	}

	return nil
}

func isSVGContentType(contentType string) bool {
	return normalizedContentType(contentType) == "image/svg+xml"
}

func normalizedContentType(contentType string) string {
	base := strings.TrimSpace(strings.ToLower(contentType))
	if semi := strings.Index(base, ";"); semi >= 0 {
		base = strings.TrimSpace(base[:semi])
	}
	return base
}
