package crawler

import (
	"regexp"
	"strings"
)

// Category represents the classification of a request for crawler controls.
type Category int

const unknownString = "unknown"

// Classification categories.
const (
	CategoryHuman Category = iota
	CategoryFederation
	CategorySearchEngine
	CategoryAICrawler
	CategoryGenericBot
	CategorySuspicious
)

func (c Category) String() string {
	switch c {
	case CategoryHuman:
		return "human"
	case CategoryFederation:
		return "federation"
	case CategorySearchEngine:
		return "search_engine"
	case CategoryAICrawler:
		return "ai_crawler"
	case CategoryGenericBot:
		return "generic_bot"
	case CategorySuspicious:
		return "suspicious"
	default:
		return unknownString
	}
}

// ClassifyRequest determines the category of a request based on user agent, accept header,
// and path. It is intentionally pure (no AWS/DB deps) so it can be used across Lambdas.
//
// Priority rule: explicit AI crawler UA matches always win and must not be bypassable by
// setting ActivityPub-ish accept headers.
func ClassifyRequest(userAgent, acceptHeader, path string) (Category, string) {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	accept := strings.ToLower(strings.TrimSpace(acceptHeader))
	pathLower := strings.ToLower(strings.TrimSpace(path))

	for _, pattern := range aiCrawlerPatterns {
		if ua != "" && strings.Contains(ua, pattern) {
			return CategoryAICrawler, "ua:" + pattern
		}
	}

	if isFederationPath(pathLower) {
		return CategoryFederation, "path:federation"
	}
	if isActivityPubAccept(accept) && isActivityPubReadPath(pathLower) {
		return CategoryFederation, "accept+path:activitypub"
	}

	for _, pattern := range searchEnginePatterns {
		if ua != "" && strings.Contains(ua, pattern) {
			return CategorySearchEngine, "ua:" + pattern
		}
	}

	for _, pattern := range allowedIntegrationBotPatterns {
		if ua != "" && strings.Contains(ua, pattern) {
			return CategoryHuman, "ua:integration:" + pattern
		}
	}

	if ua == "" || len(ua) < 10 {
		return CategorySuspicious, "ua:missing"
	}
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(ua, pattern) {
			return CategorySuspicious, "ua:" + pattern
		}
	}

	if genericBotRegex.MatchString(ua) {
		return CategoryGenericBot, "ua:generic_regex"
	}

	return CategoryHuman, "default"
}

func isActivityPubAccept(accept string) bool {
	if accept == "" {
		return false
	}
	return strings.Contains(accept, "application/activity+json") || strings.Contains(accept, "application/ld+json")
}

func isActivityPubReadPath(path string) bool {
	if path == "" {
		return false
	}
	switch {
	case strings.HasPrefix(path, "/users/"):
		return true
	case strings.HasPrefix(path, "/objects/"):
		return true
	default:
		return false
	}
}

func isFederationPath(path string) bool {
	if path == "" {
		return false
	}

	federationPrefixes := []string{
		"/.well-known/webfinger",
		"/.well-known/nodeinfo",
		"/.well-known/host-meta",
		"/.well-known/host-meta.json",
		"/nodeinfo/2.0",
		"/users/",
	}

	for _, prefix := range federationPrefixes {
		if strings.HasPrefix(path, prefix) {
			if prefix != "/users/" {
				return true
			}
			// /users/ itself is ambiguous (HTML profiles vs ActivityPub JSON). Treat only
			// the clearly federation endpoints under /users/ as federation by path.
			if strings.Contains(path, "/inbox") ||
				strings.Contains(path, "/outbox") ||
				strings.Contains(path, "/followers") ||
				strings.Contains(path, "/following") ||
				strings.Contains(path, "/liked") {
				return true
			}
		}
	}

	return false
}

var (
	aiCrawlerPatterns = []string{
		"gptbot",
		"chatgpt",
		"ccbot",
		"anthropic",
		"claude-web",
		"bytespider",
		"meta-external",
		"facebookbot",
		"facebookexternalhit",
		"amazonbot",
		"google-extended",
		"perplexitybot",
		"applebot-extended",
		"cohere-ai",
		"diffbot",
		"imagesiftbot",
		"omgili",
		"petalbot",
		"semrushbot",
		"ahrefsbot",
		"mj12bot",
		"dotbot",
		"blexbot",
		"dataforseo",
		"serpstatbot",
	}

	searchEnginePatterns = []string{
		"googlebot",
		"bingbot",
		"duckduckbot",
		"slurp", // Yahoo
		"yandexbot",
		"baiduspider",
	}

	allowedIntegrationBotPatterns = []string{
		"discordbot",
		"slackbot",
		"twitterbot",
		"telegrambot",
		"whatsapp",
		"linkedin",
	}

	suspiciousPatterns = []string{
		"python-requests",
		"curl/",
		"wget/",
		"libwww-perl",
		"java/",
		"okhttp",
		"httpunit",
		"httrack",
		"larbin",
		"nutch",
		"scrapy",
		"mechanize",
		"phantomjs",
		"headless",
		"postman",
	}

	genericBotRegex = regexp.MustCompile(`(?i)(bot|spider|crawler|scraper|fetcher|archiver)`)
)
