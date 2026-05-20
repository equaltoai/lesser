// Package cmsrender owns Lesser's canonical Article rendering boundary.
package cmsrender

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

const (
	// FormatHTML identifies already-HTML source that must be sanitized before output.
	FormatHTML = "html"
	// FormatMarkdown identifies Markdown source rendered to HTML and then sanitized.
	FormatMarkdown = "markdown"

	// MaxArticleSourceBytes caps submitted Article source before rendering.
	MaxArticleSourceBytes = 256 * 1024
	// MaxArticleRenderedHTMLBytes caps server-rendered Article HTML output.
	MaxArticleRenderedHTMLBytes = 512 * 1024
)

var (
	// ErrUnsupportedContentFormat is wrapped when an Article content format is unsupported.
	ErrUnsupportedContentFormat = errors.New("unsupported article content format")
	// ErrArticleContentTooLarge is wrapped when Article source exceeds MaxArticleSourceBytes.
	ErrArticleContentTooLarge = errors.New("article content exceeds maximum source size")
	// ErrArticleRenderedContentTooLarge is wrapped when rendered output exceeds MaxArticleRenderedHTMLBytes.
	ErrArticleRenderedContentTooLarge = errors.New("rendered article content exceeds maximum output size")

	articleClassRE     = regexp.MustCompile(`^(h-card|mention|hashtag|invisible|ellipsis|language-[A-Za-z0-9_+.-]+)$`)
	articleHeadingIDRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_:.\-]*$`)
	articleDimensionRE = regexp.MustCompile(`^[0-9]{1,5}$`)
)

// RenderedArticleContent is the public Article body produced by the canonical renderer.
type RenderedArticleContent struct {
	HTML          string
	SourceFormat  string
	SourceBytes   int
	RenderedBytes int
}

// LimitError reports deterministic, user-facing Article renderer limits.
type LimitError struct {
	Field  string
	Limit  int
	Actual int
	Cause  error
}

func (e *LimitError) Error() string {
	if e == nil {
		return "article content limit error"
	}
	field := strings.TrimSpace(e.Field)
	if field == "" {
		field = "article content"
	}
	cause := e.Cause
	if cause == nil {
		cause = ErrArticleContentTooLarge
	}
	return fmt.Sprintf("%s: %s (limit %d bytes, got %d bytes)", field, cause.Error(), e.Limit, e.Actual)
}

// Unwrap returns the sentinel limit cause.
func (e *LimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NormalizeFormat returns the storage-level content format. Empty source defaults to Markdown.
func NormalizeFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatMarkdown, "md":
		return FormatMarkdown, nil
	case FormatHTML:
		return FormatHTML, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedContentFormat, strings.TrimSpace(format))
	}
}

// ValidateArticleSource enforces source-format and source-size rules without rendering.
func ValidateArticleSource(content string, format string) error {
	if _, err := NormalizeFormat(format); err != nil {
		return err
	}
	if len(content) > MaxArticleSourceBytes {
		return &LimitError{
			Field:  "content",
			Limit:  MaxArticleSourceBytes,
			Actual: len(content),
			Cause:  ErrArticleContentTooLarge,
		}
	}
	if !utf8.ValidString(content) {
		return errors.New("article content must be valid UTF-8")
	}
	return nil
}

// RenderArticleContent renders Article source to the canonical sanitized public HTML body.
func RenderArticleContent(content string, format string) (RenderedArticleContent, error) {
	if err := ValidateArticleSource(content, format); err != nil {
		return RenderedArticleContent{}, err
	}

	normalizedFormat, _ := NormalizeFormat(format)

	var rendered string
	var err error
	switch normalizedFormat {
	case FormatHTML:
		rendered = SanitizeArticleHTML(content)
	case FormatMarkdown:
		rendered, err = renderMarkdownToHTML(content)
		if err != nil {
			return RenderedArticleContent{}, err
		}
		rendered = SanitizeArticleHTML(rendered)
	default:
		return RenderedArticleContent{}, fmt.Errorf("%w: %s", ErrUnsupportedContentFormat, normalizedFormat)
	}

	if len(rendered) > MaxArticleRenderedHTMLBytes {
		return RenderedArticleContent{}, &LimitError{
			Field:  "rendered_content",
			Limit:  MaxArticleRenderedHTMLBytes,
			Actual: len(rendered),
			Cause:  ErrArticleRenderedContentTooLarge,
		}
	}

	return RenderedArticleContent{
		HTML:          rendered,
		SourceFormat:  normalizedFormat,
		SourceBytes:   len(content),
		RenderedBytes: len(rendered),
	}, nil
}

// SanitizeArticleHTML sanitizes Article HTML source/output using the CMS publication policy.
func SanitizeArticleHTML(content string) string {
	if content == "" {
		return ""
	}
	return articlePolicy().Sanitize(content)
}

func renderMarkdownToHTML(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(ghtml.WithUnsafe()),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return "", fmt.Errorf("render article markdown: %w", err)
	}
	return buf.String(), nil
}

func articlePolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.RequireParseableURLs(true)
	policy.AllowURLSchemes("http", "https", "mailto")

	policy.AllowElements(
		"p", "br", "span", "del", "pre", "code", "em", "strong", "b", "i", "u", "s", "strike",
		"blockquote", "ul", "ol", "li", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "a", "img",
		"table", "thead", "tbody", "tr", "th", "td",
	)

	policy.AllowAttrs("id").Matching(articleHeadingIDRE).OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	policy.AllowAttrs("class").Matching(articleClassRE).OnElements("span", "a", "code")
	policy.AllowAttrs("href", "title", "rel").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	policy.AllowAttrs("src").OnElements("img")
	policy.AllowAttrs("alt", "title").OnElements("img")
	policy.AllowAttrs("width", "height").Matching(articleDimensionRE).OnElements("img")
	policy.AllowAttrs("data-user").OnElements("span", "a")

	return policy
}
