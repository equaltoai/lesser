package cmsrender

import (
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ArticleMediaRole describes how one bound editorial asset composes into
// article HTML. The canonical renderer treats the role as presentation
// semantics only: hero leads the article, inline inserts at a zero-based block
// position, and social-card media is never emitted into the body.
type ArticleMediaRole string

const (
	// ArticleMediaRoleHero is the article's leading image.
	ArticleMediaRoleHero ArticleMediaRole = "hero"
	// ArticleMediaRoleInline places an image at a modeled block position.
	ArticleMediaRoleInline ArticleMediaRole = "inline"
	// ArticleMediaRoleSocialCard is the promotional card presentation; it does
	// not compose into the article body.
	ArticleMediaRoleSocialCard ArticleMediaRole = "social_card"
)

// ArticleMedia is one composed editorial image descriptor. The URL must be a
// minted published serving (durable) or a caller-authorized short-lived serving
// (draft preview); the renderer never mints or resolves URLs and only emits the
// exact descriptors it is given.
type ArticleMedia struct {
	// Role selects the placement semantics (hero, inline, social_card).
	Role ArticleMediaRole
	// InlinePosition is the zero-based insertion point for inline media: the
	// figure is inserted before the Nth top-level block of the rendered
	// article. Positions at or past the block count append at the end.
	InlinePosition int
	URL            string
	AltText        string
	Caption        string
	CreditLine     string
	Width          int
	Height         int
	ContentType    string
}

// placedArticleMedia is a resolved insertion: a figure plus the block index it
// leads. hero figures always sort first.
type placedArticleMedia struct {
	position int
	hero     bool
	figure   *html.Node
}

// composeArticleMedia inserts the article's media descriptors into sanitized
// block HTML. Hero media leads the article; inline media inserts before the
// top-level block at its zero-based position; social-card media is ignored.
// The input is expected to be sanitized (balanced) HTML; the composed result is
// re-sanitized by the caller.
func composeArticleMedia(sanitizedHTML string, media []ArticleMedia) string {
	placements := buildArticleMediaPlacements(media)
	if len(placements) == 0 {
		return sanitizedHTML
	}

	doc, err := html.Parse(strings.NewReader(sanitizedHTML))
	if err != nil {
		return sanitizedHTML
	}
	body := htmlBodyNode(doc)
	if body == nil {
		return sanitizedHTML
	}

	blocks := topLevelBlockNodes(body)
	byPosition := groupPlacementsByPosition(placements, len(blocks))

	// Insert before each block, from the end backwards so earlier positions stay
	// stable, then append placements whose position is past the block count.
	for i := len(blocks) - 1; i >= 0; i-- {
		for _, p := range byPosition[i] {
			body.InsertBefore(p.figure, blocks[i])
		}
	}
	for _, p := range byPosition[len(blocks)] {
		body.AppendChild(p.figure)
	}

	var buf strings.Builder
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buf, child); err != nil {
			continue
		}
	}
	return buf.String()
}

// buildArticleMediaPlacements orders media into stable insertion slots. Only
// hero and inline roles compose; inline entries sort by position so authored
// order never depends on list order, and hero always leads.
func buildArticleMediaPlacements(media []ArticleMedia) []placedArticleMedia {
	placements := make([]placedArticleMedia, 0, len(media))
	for _, m := range media {
		switch m.Role {
		case ArticleMediaRoleHero:
			placements = append(placements, placedArticleMedia{position: 0, hero: true, figure: buildArticleFigure(m)})
		case ArticleMediaRoleInline:
			position := m.InlinePosition
			if position < 0 {
				position = 0
			}
			placements = append(placements, placedArticleMedia{position: position, figure: buildArticleFigure(m)})
		default:
			// Social-card and unknown roles never compose into the body.
		}
	}
	sort.SliceStable(placements, func(i, j int) bool {
		if placements[i].hero != placements[j].hero {
			return placements[i].hero
		}
		return placements[i].position < placements[j].position
	})
	return placements
}

// groupPlacementsByPosition buckets placements by insertion slot. Slots are
// clamped to the block count so out-of-range positions append at the end.
func groupPlacementsByPosition(placements []placedArticleMedia, blockCount int) map[int][]placedArticleMedia {
	grouped := make(map[int][]placedArticleMedia, blockCount+1)
	for _, p := range placements {
		slot := p.position
		if slot > blockCount {
			slot = blockCount
		}
		grouped[slot] = append(grouped[slot], p)
	}
	return grouped
}

// htmlBodyNode returns the parsed document's body element, or nil.
func htmlBodyNode(doc *html.Node) *html.Node {
	if doc == nil {
		return nil
	}
	var walk func(*html.Node) *html.Node
	walk = func(n *html.Node) *html.Node {
		if n == nil {
			return nil
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "body") {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(doc)
}

// topLevelBlockNodes returns the top-level block elements of a parsed body.
// Inter-block whitespace text nodes do not consume a position.
func topLevelBlockNodes(body *html.Node) []*html.Node {
	if body == nil {
		return nil
	}
	blocks := make([]*html.Node, 0, 8)
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && isArticleBlockElement(child.Data) {
			blocks = append(blocks, child)
		}
	}
	return blocks
}

// articleBlockElements are the block-level elements the publication policy can
// emit at the top level of an article body; these are the positions inline
// media inserts between. Inline-level and inter-block whitespace nodes never
// consume a position.
var articleBlockElements = map[string]struct{}{
	"p": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"ul": {}, "ol": {}, "blockquote": {}, "pre": {}, "table": {}, "hr": {},
	"figure": {}, "figcaption": {},
}

func isArticleBlockElement(data string) bool {
	_, ok := articleBlockElements[strings.ToLower(strings.TrimSpace(data))]
	return ok
}

// buildArticleFigure constructs the <figure>/<img> node for one media
// descriptor. Alt, caption, and credit are inserted as text (never parsed as
// HTML); width/height are emitted only when positive and within the sanitizer's
// dimension range. The caption and reader-facing credit line share the
// figcaption when either is present.
func buildArticleFigure(m ArticleMedia) *html.Node {
	figure := &html.Node{Type: html.ElementNode, Data: "figure"}

	img := &html.Node{Type: html.ElementNode, Data: "img"}
	img.Attr = append(img.Attr, html.Attribute{Key: "src", Val: m.URL})
	img.Attr = append(img.Attr, html.Attribute{Key: "alt", Val: m.AltText})
	if width, ok := articleDimensionAttr(m.Width); ok {
		img.Attr = append(img.Attr, html.Attribute{Key: "width", Val: width})
	}
	if height, ok := articleDimensionAttr(m.Height); ok {
		img.Attr = append(img.Attr, html.Attribute{Key: "height", Val: height})
	}
	figure.AppendChild(img)

	if text := articleFigcaptionText(m); text != "" {
		figcaption := &html.Node{Type: html.ElementNode, Data: "figcaption"}
		figcaption.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		figure.AppendChild(figcaption)
	}

	return figure
}

// articleFigcaptionText joins the caption and reader-facing credit line into
// the figcaption text. Credit alone renders as its own figcaption so a
// commissioned piece without a caption still carries its attribution.
func articleFigcaptionText(m ArticleMedia) string {
	caption := strings.TrimSpace(m.Caption)
	credit := strings.TrimSpace(m.CreditLine)
	switch {
	case caption == "":
		return credit
	case credit == "":
		return caption
	default:
		return caption + " — " + credit
	}
}

// articleDimensionAttr serializes a positive image dimension within the
// sanitizer's [0-9]{1,5} range. Zero and out-of-range values are omitted.
func articleDimensionAttr(value int) (string, bool) {
	if value <= 0 || value > 99999 {
		return "", false
	}
	return strconv.Itoa(value), true
}
