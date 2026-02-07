package headers

import (
	"fmt"
	"strings"
)

// StaticHTMLPageCSP returns a strict CSP suitable for simple HTML pages that do not require scripts.
func StaticHTMLPageCSP() string {
	return "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; img-src https: http: data:; style-src 'unsafe-inline'; script-src 'none'; object-src 'none'"
}

// EmbedHTMLPageCSP returns a strict CSP for embeddable HTML pages that require a single inline script.
// The provided nonce is used to allow that script without enabling unsafe-inline.
func EmbedHTMLPageCSP(scriptNonce string) string {
	scriptNonce = strings.TrimSpace(scriptNonce)
	if scriptNonce == "" {
		return "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors *; img-src https: http: data:; style-src 'unsafe-inline'; script-src 'none'; object-src 'none'"
	}

	scriptNonceValue := fmt.Sprintf("'nonce-%s'", scriptNonce)
	return fmt.Sprintf("default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors *; img-src https: http: data:; style-src 'unsafe-inline'; script-src %s; object-src 'none'", scriptNonceValue)
}
