package cms

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"golang.org/x/net/html"
)

var (
	cmsHTMLTagsRE        = regexp.MustCompile(`<[^>]*>`)
	cmsMarkdownLinkRE    = regexp.MustCompile(`!?\[([^\]]+)\]\([^)]+\)`)
	cmsMarkdownCodeSpan  = regexp.MustCompile("`[^`]*`")
	cmsMarkdownEmphasis  = regexp.MustCompile(`[*_]{1,3}`)
	cmsMarkdownHeadingRE = regexp.MustCompile(`^(#{1,6})\s+(.+?)(?:\s+#+\s*)?$`)
)

func enrichArticleContent(article *models.Article) {
	if article == nil {
		return
	}

	content := strings.TrimSpace(article.Content)
	format := strings.ToLower(strings.TrimSpace(article.ContentFormat))

	article.WordCount = cmsCountWords(content, format)
	article.ReadingTimeMinutes = cmsEstimateReadingMinutes(article.WordCount)
	article.TableOfContents = cmsExtractTOC(content, format)
}

func cmsEstimateReadingMinutes(wordCount int) int {
	if wordCount <= 0 {
		return 0
	}

	const wordsPerMinute = 200
	minutes := (wordCount + (wordsPerMinute - 1)) / wordsPerMinute
	if minutes < 1 {
		return 1
	}
	return minutes
}

func cmsCountWords(content string, format string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}

	plain := ""
	switch format {
	case "html":
		plain = cmsStripHTML(content)
	default:
		plain = cmsStripMarkdown(content)
	}

	plain = strings.TrimSpace(plain)
	if plain == "" {
		return 0
	}
	return len(strings.Fields(plain))
}

func cmsStripHTML(content string) string {
	// Fast path: strip tags. This is approximate but efficient and avoids heavy parsing for word counts.
	return cmsHTMLTagsRE.ReplaceAllString(content, " ")
}

func cmsStripMarkdown(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var out strings.Builder
	out.Grow(len(content))

	inCodeBlock := false
	codeFence := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if fence, ok := cmsMarkdownFence(trimmed); ok {
			if inCodeBlock && fence == codeFence {
				inCodeBlock = false
				codeFence = ""
			} else if !inCodeBlock {
				inCodeBlock = true
				codeFence = fence
			}
			continue
		}
		if inCodeBlock {
			continue
		}

		// Remove common heading and list markers.
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#>-*+ \t"))
		out.WriteString(trimmed)
		out.WriteByte('\n')
	}

	text := out.String()
	text = cmsMarkdownLinkRE.ReplaceAllString(text, "$1")
	text = cmsMarkdownCodeSpan.ReplaceAllString(text, " ")
	text = cmsMarkdownEmphasis.ReplaceAllString(text, "")
	text = cmsStripHTML(text)
	return text
}

func cmsMarkdownFence(trimmedLine string) (string, bool) {
	if strings.HasPrefix(trimmedLine, "```") {
		return "```", true
	}
	if strings.HasPrefix(trimmedLine, "~~~") {
		return "~~~", true
	}
	return "", false
}

func cmsExtractTOC(content string, format string) []models.TOCEntry {
	content = strings.TrimSpace(content)
	if content == "" {
		return []models.TOCEntry{}
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "html":
		return cmsExtractTOCFromHTML(content)
	default:
		return cmsExtractTOCFromMarkdown(content)
	}
}

func cmsExtractTOCFromMarkdown(content string) []models.TOCEntry {
	scanner := bufio.NewScanner(strings.NewReader(content))

	used := map[string]int{}
	entries := make([]models.TOCEntry, 0, 8)

	inCodeBlock := false
	codeFence := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if fence, ok := cmsMarkdownFence(trimmed); ok {
			if inCodeBlock && fence == codeFence {
				inCodeBlock = false
				codeFence = ""
			} else if !inCodeBlock {
				inCodeBlock = true
				codeFence = fence
			}
			continue
		}
		if inCodeBlock {
			continue
		}

		match := cmsMarkdownHeadingRE.FindStringSubmatch(trimmed)
		if len(match) == 0 {
			continue
		}

		level := len(match[1])
		text := cmsNormalizeHeadingText(match[2])
		if text == "" {
			continue
		}

		id := cmsUniqueHeadingID(common.Slugify(text), used)
		if id == "" {
			continue
		}

		entries = append(entries, models.TOCEntry{
			ID:    id,
			Level: level,
			Text:  text,
		})
	}

	return entries
}

func cmsNormalizeHeadingText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	text = cmsMarkdownLinkRE.ReplaceAllString(text, "$1")
	text = cmsMarkdownCodeSpan.ReplaceAllString(text, " ")
	text = cmsMarkdownEmphasis.ReplaceAllString(text, "")
	text = cmsStripHTML(text)

	// Collapse whitespace.
	return strings.Join(strings.Fields(text), " ")
}

func cmsUniqueHeadingID(base string, used map[string]int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}

	if used == nil {
		return base
	}

	count := used[base]
	if count == 0 {
		used[base] = 1
		return base
	}

	count++
	used[base] = count
	return base + "-" + strconv.Itoa(count)
}

func cmsExtractTOCFromHTML(content string) []models.TOCEntry {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return []models.TOCEntry{}
	}

	used := map[string]int{}
	entries := make([]models.TOCEntry, 0, 8)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}

		if n.Type == html.ElementNode {
			if level, ok := cmsHeadingLevel(n.Data); ok {
				text := cmsHTMLText(n)
				text = strings.Join(strings.Fields(text), " ")
				if text != "" {
					id := strings.TrimSpace(cmsGetHTMLAttr(n, "id"))
					if id == "" {
						id = common.Slugify(text)
					}
					id = cmsUniqueHeadingID(id, used)
					if id != "" {
						entries = append(entries, models.TOCEntry{
							ID:    id,
							Level: level,
							Text:  text,
						})
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return entries
}

func cmsHeadingLevel(tag string) (int, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if len(tag) != 2 || tag[0] != 'h' {
		return 0, false
	}

	level, err := strconv.Atoi(tag[1:])
	if err != nil || level < 1 || level > 6 {
		return 0, false
	}
	return level, true
}

func cmsGetHTMLAttr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func cmsHTMLText(n *html.Node) string {
	var buf bytes.Buffer
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}

		switch node.Type {
		case html.TextNode:
			buf.WriteString(node.Data)
			buf.WriteByte(' ')
			return
		case html.ElementNode:
			switch strings.ToLower(node.Data) {
			case "script", "style":
				return
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return buf.String()
}
