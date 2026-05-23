package handlers

const (
	// ErrInsufficientScope is returned when the OAuth token has insufficient scope
	ErrInsufficientScope = "insufficient scope"

	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// dangerousURLSchemes lists URL schemes that must never appear in actor
// identifier or URL fields, even when the value is not expected to be a
// well-formed URL (e.g. email-address placeholders).
var dangerousURLSchemes = []string{
	"javascript",
	"data",
	"vbscript",
}
