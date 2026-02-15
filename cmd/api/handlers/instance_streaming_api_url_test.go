package handlers

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_streamingAPIURL_round28_more_coverage(t *testing.T) {
	t.Parallel()

	t.Run("nil_handler", func(t *testing.T) {
		var h *Handler
		assert.Equal(t, "", h.streamingAPIURL())
	})

	t.Run("nil_config", func(t *testing.T) {
		h := &Handler{cfg: nil}
		assert.Equal(t, "", h.streamingAPIURL())
	})

	t.Run("explicit_wss_endpoint", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{WebSocketEndpoint: "wss://ws.example.com"}}
		assert.Equal(t, "wss://ws.example.com", h.streamingAPIURL())
	})

	t.Run("explicit_ws_endpoint", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{WebSocketEndpoint: "ws://ws.example.com"}}
		assert.Equal(t, "ws://ws.example.com", h.streamingAPIURL())
	})

	t.Run("https_endpoint_normalizes_to_wss", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{WebSocketEndpoint: "https://ws.example.com"}}
		assert.Equal(t, "wss://ws.example.com", h.streamingAPIURL())
	})

	t.Run("http_endpoint_normalizes_to_ws", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{WebSocketEndpoint: "http://ws.example.com"}}
		assert.Equal(t, "ws://ws.example.com", h.streamingAPIURL())
	})

	t.Run("unknown_endpoint_falls_back_to_domain", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{WebSocketEndpoint: "ftp://ws.example.com", Domain: "example.com"}}
		assert.Equal(t, "wss://ws.example.com", h.streamingAPIURL())
	})

	t.Run("default_domain_uses_wss", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "example.com"}}
		assert.Equal(t, "wss://ws.example.com", h.streamingAPIURL())
	})

	t.Run("localhost_domain_uses_ws", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "localhost"}}
		assert.Equal(t, "ws://ws.localhost", h.streamingAPIURL())
	})

	t.Run("loopback_domain_uses_ws", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "127.0.0.1"}}
		assert.Equal(t, "ws://ws.127.0.0.1", h.streamingAPIURL())
	})
}

func TestIsProductionEnvironment_round28_more_coverage(t *testing.T) {
	t.Parallel()

	assert.True(t, IsProductionEnvironment(&config.Config{Stage: EnvProduction}))
	assert.True(t, IsProductionEnvironment(&config.Config{Stage: EnvProd}))
	assert.False(t, IsProductionEnvironment(&config.Config{Stage: "test"}))
}

func TestGenerateVAPIDKeyPair_round28_more_coverage(t *testing.T) {
	t.Parallel()

	publicKeyB64, privateKeyPEM, err := generateVAPIDKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, publicKeyB64)
	require.NotEmpty(t, privateKeyPEM)

	block, _ := pem.Decode([]byte(privateKeyPEM))
	require.NotNil(t, block)
	_, err = x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)
}

func TestHandler_markdownToHTMLLift_round28_more_coverage(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	out := h.markdownToHTMLLift("# Title\n\nHello\nworld\n\n## Subtitle\nParagraph2")

	assert.Contains(t, out, "<h1>Title</h1>")
	assert.Contains(t, out, "<p>Hello")
	assert.Contains(t, out, "world")
	assert.Contains(t, out, "<h2>Subtitle</h2>")
	assert.Contains(t, out, "<p>Paragraph2")
	assert.True(t, strings.HasSuffix(out, "</p>"))
}

func TestHandler_processMarkdownLine_round28_more_coverage(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("header_closes_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("# Title", "# Title", true)
		assert.Equal(t, "</p>\n<h1>Title</h1>", html)
		assert.False(t, inParagraph)
	})

	t.Run("header_outside_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("## Subtitle", "## Subtitle", false)
		assert.Equal(t, "<h2>Subtitle</h2>", html)
		assert.False(t, inParagraph)
	})

	t.Run("h3_header_outside_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("### Minor", "### Minor", false)
		assert.Equal(t, "<h3>Minor</h3>", html)
		assert.False(t, inParagraph)
	})

	t.Run("empty_line_closes_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("", "", true)
		assert.Equal(t, "</p>", html)
		assert.False(t, inParagraph)
	})

	t.Run("empty_line_outside_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("", "", false)
		assert.Equal(t, "", html)
		assert.False(t, inParagraph)
	})

	t.Run("regular_text_starts_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("hello", "hello", false)
		assert.Equal(t, "<p>hello", html)
		assert.True(t, inParagraph)
	})

	t.Run("regular_text_continues_paragraph", func(t *testing.T) {
		html, inParagraph := h.processMarkdownLine("world", "world", true)
		assert.Equal(t, "world", html)
		assert.True(t, inParagraph)
	})
}
