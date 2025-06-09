package common

import (
	"net/http"
	"time"
)

// CookieConfig holds configuration for secure cookies
type CookieConfig struct {
	Domain   string // Leave empty to let browser handle
	Path     string
	Secure   bool // HTTPS only
	HttpOnly bool // No JavaScript access
	SameSite http.SameSite
}

// DefaultCookieConfig provides secure defaults
var DefaultCookieConfig = CookieConfig{
	Domain:   "", // Browser handles domain
	Path:     "/",
	Secure:   true,
	HttpOnly: true,
	SameSite: http.SameSiteStrictMode, // Maximum CSRF protection
}

// SetSecureCookie sets a cookie with all security flags
func SetSecureCookie(w http.ResponseWriter, name, value string, maxAge int) {
	SetSecureCookieWithConfig(w, name, value, maxAge, DefaultCookieConfig)
}

// SetSecureCookieWithConfig sets a cookie with custom configuration
func SetSecureCookieWithConfig(w http.ResponseWriter, name, value string, maxAge int, config CookieConfig) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     config.Path,
		Domain:   config.Domain,
		MaxAge:   maxAge,
		Secure:   config.Secure,
		HttpOnly: config.HttpOnly,
		SameSite: config.SameSite,
	}

	// If maxAge is negative, set expires to delete cookie
	if maxAge < 0 {
		cookie.Expires = time.Unix(0, 0)
	}

	http.SetCookie(w, cookie)
}

// DeleteSecureCookie removes a cookie securely
func DeleteSecureCookie(w http.ResponseWriter, name string) {
	SetSecureCookie(w, name, "", -1)
}

// ParseCookies extracts cookies from request headers (Lambda compatible)
func ParseCookies(cookieHeader string) map[string]string {
	cookies := make(map[string]string)

	if cookieHeader == "" {
		return cookies
	}

	// Parse cookie header
	header := http.Header{}
	header.Add("Cookie", cookieHeader)
	request := &http.Request{Header: header}

	for _, cookie := range request.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	return cookies
}

// GetCookie retrieves a specific cookie value from headers
func GetCookie(headers map[string]string, name string) string {
	cookieHeader := headers["Cookie"]
	if cookieHeader == "" {
		cookieHeader = headers["cookie"]
	}

	cookies := ParseCookies(cookieHeader)
	return cookies[name]
}
