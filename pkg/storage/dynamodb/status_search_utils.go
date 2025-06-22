package dynamodb

import (
	"regexp"
	"strings"
)

var (
	// Regular expressions for parsing queries
	hashtagRegex = regexp.MustCompile(`#[a-zA-Z0-9_]+`)
	mentionRegex = regexp.MustCompile(`@[a-zA-Z0-9_]+(@[a-zA-Z0-9.-]+)?`)
	urlRegex     = regexp.MustCompile(`https?://[^\s]+`)

	// Common stop words for filtering
	stopWords = map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
		"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
		"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
		"that": true, "the": true, "to": true, "was": true, "will": true, "with": true,
		"but": true, "or": true, "not": true, "this": true, "have": true, "had": true,
		"what": true, "when": true, "where": true, "who": true, "which": true, "why": true,
		"how": true, "all": true, "each": true, "every": true, "some": true, "any": true,
		"i": true, "me": true, "my": true, "we": true, "our": true, "you": true, "your": true,
		"they": true, "them": true, "their": true, "she": true, "her": true, "him": true,
		"his": true, "about": true, "against": true, "between": true, "into": true,
		"through": true, "during": true, "before": true, "after": true, "above": true,
		"below": true, "up": true, "down": true, "out": true, "off": true, "over": true,
		"under": true, "again": true, "further": true, "then": true, "once": true,
	}
)

// isStopWord checks if a word is a common stop word
func isStopWord(word string) bool {
	return stopWords[strings.ToLower(word)]
}

// extractSignificantWords extracts meaningful words from text for indexing
func extractSignificantWords(content string) []string {
	// Normalize content
	content = strings.ToLower(content)

	// Remove URLs
	content = urlRegex.ReplaceAllString(content, "")

	// Remove mentions (we index those separately)
	content = mentionRegex.ReplaceAllString(content, "")

	// Remove hashtags (indexed separately)
	content = hashtagRegex.ReplaceAllString(content, "")

	// Split into words
	words := strings.FieldsFunc(content, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})

	// Filter out stop words and short words
	significant := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) < 3 || isStopWord(word) || seen[word] {
			continue
		}
		seen[word] = true
		significant = append(significant, word)

		// Limit to top 20 words as per plan
		if len(significant) >= 20 {
			break
		}
	}

	return significant
}

// extractHashtags extracts hashtags from content
func extractHashtags(content string) []string {
	matches := hashtagRegex.FindAllString(content, -1)
	hashtags := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		// Remove # prefix and lowercase
		tag := strings.ToLower(strings.TrimPrefix(match, "#"))
		if !seen[tag] {
			seen[tag] = true
			hashtags = append(hashtags, tag)
		}
	}

	return hashtags
}

// extractMentions extracts mentions from content
func extractMentions(content string) []string {
	matches := mentionRegex.FindAllString(content, -1)
	mentions := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		// Remove @ prefix
		mention := strings.TrimPrefix(match, "@")
		if !seen[mention] {
			seen[mention] = true
			mentions = append(mentions, mention)
		}
	}

	return mentions
}

// extractURLs extracts URLs from content
func extractURLs(content string) []string {
	matches := urlRegex.FindAllString(content, -1)
	urls := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, url := range matches {
		if !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}

	return urls
}


// highlightMatch creates a highlighted version of text with the match emphasized
func highlightMatch(text, query string, maxLength int) string {
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	index := strings.Index(lowerText, lowerQuery)
	if index == -1 {
		// No match, return truncated text
		if len(text) > maxLength {
			return text[:maxLength] + "..."
		}
		return text
	}

	// Calculate excerpt boundaries
	start := index - 50
	if start < 0 {
		start = 0
	}

	end := index + len(query) + 100
	if end > len(text) {
		end = len(text)
	}

	// Adjust to word boundaries
	if start > 0 {
		// Find previous space
		for i := start; i < index && i < len(text); i++ {
			if text[i] == ' ' {
				start = i + 1
				break
			}
		}
	}

	if end < len(text) {
		// Find next space
		for i := end; i > index+len(query) && i > 0; i-- {
			if text[i-1] == ' ' {
				end = i - 1
				break
			}
		}
	}

	// Build highlighted excerpt
	excerpt := ""
	if start > 0 {
		excerpt = "..."
	}

	// Add text before match
	excerpt += text[start:index]

	// Add highlighted match
	excerpt += "<em>" + text[index:index+len(query)] + "</em>"

	// Add text after match
	excerpt += text[index+len(query) : end]

	if end < len(text) {
		excerpt += "..."
	}

	return excerpt
}
