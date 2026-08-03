package crawler

import (
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

// RobotsTxt is the robots.txt policy for Lesser instances.
//
// It is intentionally restrictive by default to reduce bot-driven costs, while
// allowing common search engines at low rates and preserving human access.
const RobotsTxt = `# Lesser ActivityPub Instance
# https://github.com/lesser-project/lesser

# Legitimate search engines - allowed with crawl-delay
User-agent: Googlebot
Crawl-delay: 10
Allow: /

User-agent: Bingbot
Crawl-delay: 10
Allow: /

User-agent: DuckDuckBot
Crawl-delay: 10
Allow: /

User-agent: Slurp
Crawl-delay: 10
Allow: /

# AI Training Crawlers - DISALLOWED
User-agent: GPTBot
Disallow: /

User-agent: ChatGPT-User
Disallow: /

User-agent: CCBot
Disallow: /

User-agent: anthropic-ai
Disallow: /

User-agent: Claude-Web
Disallow: /

User-agent: Bytespider
Disallow: /

User-agent: Meta-ExternalAgent
Disallow: /

User-agent: Meta-ExternalFetcher
Disallow: /

User-agent: FacebookBot
Disallow: /

User-agent: Amazonbot
Disallow: /

User-agent: Google-Extended
Disallow: /

User-agent: PerplexityBot
Disallow: /

User-agent: Applebot-Extended
Disallow: /

User-agent: cohere-ai
Disallow: /

User-agent: Diffbot
Disallow: /

User-agent: ImagesiftBot
Disallow: /

User-agent: Omgilibot
Disallow: /

User-agent: Omgili
Disallow: /

# Default - be restrictive (avoid API and auth surfaces)
User-agent: *
Crawl-delay: 30
Disallow: /api/
Disallow: /oauth/
Disallow: /.well-known/webfinger
Allow: /users/
Allow: /objects/
`

// RobotsHandler returns robots.txt with aggressive caching.
func RobotsHandler(*apptheory.Context) (*apptheory.Response, error) {
	resp := apptheory.Text(200, RobotsTxt)
	resp.Headers = map[string][]string{
		"content-type":  {"text/plain; charset=utf-8"},
		"cache-control": {"public, max-age=86400"}, // 24 hour cache
		"x-robots-tag":  {"noindex"},
	}
	return resp, nil
}
