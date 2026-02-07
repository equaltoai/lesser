package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstanceMarkdownHelpers_Round19(t *testing.T) {
	h, _, _ := round11NewHandler(t, nil, &round10QueryState{})

	require.Equal(t, "<h1>Title</h1>", h.convertMarkdownHeader("# Title"))
	require.Equal(t, "<h2>Title</h2>", h.convertMarkdownHeader("## Title"))
	require.Equal(t, "<h3>Title</h3>", h.convertMarkdownHeader("### Title"))
	require.Empty(t, h.convertMarkdownHeader("not a header"))

	out, inParagraph := h.processMarkdownLine("Paragraph", "Paragraph", false)
	require.Equal(t, "<p>Paragraph", out)
	require.True(t, inParagraph)

	out, inParagraph = h.processMarkdownLine("Still in paragraph", "Still in paragraph", true)
	require.Equal(t, "Still in paragraph", out)
	require.True(t, inParagraph)

	out, inParagraph = h.processMarkdownLine("", "", true)
	require.Equal(t, "</p>", out)
	require.False(t, inParagraph)

	out, inParagraph = h.processMarkdownLine("", "", false)
	require.Empty(t, out)
	require.False(t, inParagraph)

	out, inParagraph = h.processMarkdownLine("# Title", "# Title", true)
	require.Equal(t, "</p>\n<h1>Title</h1>", out)
	require.False(t, inParagraph)

	out, inParagraph = h.processMarkdownLine("## Section", "## Section", false)
	require.Equal(t, "<h2>Section</h2>", out)
	require.False(t, inParagraph)

	paragraphOnly := h.markdownToHTMLLift("Just text")
	require.Contains(t, paragraphOnly, "<p>Just text")
	require.Contains(t, paragraphOnly, "</p>")

	withHeaderMidParagraph := h.markdownToHTMLLift("First line\n# Title\nSecond line")
	require.Contains(t, withHeaderMidParagraph, "<p>First line")
	require.Contains(t, withHeaderMidParagraph, "</p>\n<h1>Title</h1>")
	require.Contains(t, withHeaderMidParagraph, "<p>Second line")
	require.Contains(t, withHeaderMidParagraph, "</p>")
}
