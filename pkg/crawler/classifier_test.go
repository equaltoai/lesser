package crawler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyRequest(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		accept   string
		path     string
		category Category
	}{
		{
			name:     "gptbot blocked even with activitypub accept",
			ua:       "GPTBot/1.0 (+https://openai.com/gptbot)",
			accept:   "application/activity+json",
			path:     "/users/alice",
			category: CategoryAICrawler,
		},
		{
			name:     "meta external agent blocked",
			ua:       "Meta-ExternalAgent/1.1 (+https://www.facebook.com/externalhit_uatext.php)",
			accept:   "text/html",
			path:     "/users/alice",
			category: CategoryAICrawler,
		},
		{
			name:     "mastodon federation by accept+path",
			ua:       "Mastodon/4.0.2",
			accept:   "application/activity+json",
			path:     "/users/alice",
			category: CategoryFederation,
		},
		{
			name:     "generic ua federation by accept+objects path",
			ua:       "Go-http-client/1.1",
			accept:   "application/activity+json",
			path:     "/objects/123",
			category: CategoryFederation,
		},
		{
			name:     "webfinger is federation by path",
			ua:       "Mozilla/5.0",
			accept:   "*/*",
			path:     "/.well-known/webfinger",
			category: CategoryFederation,
		},
		{
			name:     "googlebot search engine",
			ua:       "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			accept:   "text/html",
			path:     "/users/alice",
			category: CategorySearchEngine,
		},
		{
			name:     "slackbot allowed integration bot treated as human",
			ua:       "Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
			accept:   "text/html",
			path:     "/",
			category: CategoryHuman,
		},
		{
			name:     "curl is suspicious",
			ua:       "curl/8.4.0",
			accept:   "*/*",
			path:     "/users/alice",
			category: CategorySuspicious,
		},
		{
			name:     "empty user-agent is suspicious",
			ua:       "",
			accept:   "",
			path:     "/",
			category: CategorySuspicious,
		},
		{
			name:     "generic scraper bot",
			ua:       "ExampleScraperBot/1.0",
			accept:   "text/html",
			path:     "/",
			category: CategoryGenericBot,
		},
		{
			name:     "normal browser is human",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			accept:   "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			path:     "/",
			category: CategoryHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, _ := ClassifyRequest(tt.ua, tt.accept, tt.path)
			require.Equal(t, tt.category, category)
		})
	}
}

func TestCategoryString(t *testing.T) {
	require.Equal(t, "human", CategoryHuman.String())
	require.Equal(t, "federation", CategoryFederation.String())
	require.Equal(t, "search_engine", CategorySearchEngine.String())
	require.Equal(t, "ai_crawler", CategoryAICrawler.String())
	require.Equal(t, "generic_bot", CategoryGenericBot.String())
	require.Equal(t, "suspicious", CategorySuspicious.String())
	require.Equal(t, "unknown", Category(999).String())
}
