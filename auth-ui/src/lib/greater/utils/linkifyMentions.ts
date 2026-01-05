export interface LinkifyOptions {
	/**
	 * Base URL for user mentions (e.g., "https://mastodon.social/@")
	 */
	mentionBaseUrl?: string;
	/**
	 * Base URL for hashtags (e.g., "https://mastodon.social/tags/")
	 */
	hashtagBaseUrl?: string;
	/**
	 * CSS class for mention links
	 */
	mentionClass?: string;
	/**
	 * CSS class for hashtag links
	 */
	hashtagClass?: string;
	/**
	 * CSS class for URL links
	 */
	urlClass?: string;
	/**
	 * Whether to open links in new tab
	 */
	openInNewTab?: boolean;
	/**
	 * Maximum length for URL display text
	 */
	maxUrlLength?: number;
	/**
	 * Whether to add rel="nofollow" to links
	 */
	nofollow?: boolean;
}

/**
 * Regular expressions for matching patterns
 */
const PATTERNS = {
	// Matches @username@domain.com or @username
	mention: /(?:^|\s)(@[\w\-.]+(?:@[\w\-.]+)?)/g,
	// Matches #hashtag (with Unicode support)
	hashtag: /(?:^|\s)(#[\p{L}\p{N}_]+)/gu,
	// Matches URLs (simplified pattern)
	url: /(?:^|\s)((?:https?:\/\/)?(?:[\w-]+\.)+[a-z]{2,}(?:\/[^\s]*)?)/gi,
	// Email pattern
	email: /(?:^|\s)([\w\-.]+@[\w\-.]+\.[a-z]{2,})/gi,
};

/**
 * Escape HTML special characters
 */
const htmlEscapeMap: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'>': '&gt;',
	'"': '&quot;',
	"'": '&#39;',
};

function escapeHtml(text: string): string {
	return text.replace(/[&<>"']/g, (char) => htmlEscapeMap[char] ?? char);
}

/**
 * Truncate URL for display
 */
function truncateUrl(url: string, maxLength: number): string {
	if (url.length <= maxLength) return url;

	const urlObj = new URL(url);
	const domain = urlObj.hostname;

	if (domain.length >= maxLength - 3) {
		return domain.substring(0, maxLength - 3) + '...';
	}

	const pathLength = maxLength - domain.length - 3;
	const path = urlObj.pathname + urlObj.search + urlObj.hash;

	if (path.length <= pathLength) {
		return domain + path;
	}

	return domain + path.substring(0, pathLength) + '...';
}

/**
 * Convert mentions, hashtags, and URLs in text to clickable links
 * @param text - The text to linkify
 * @param options - Linkify options
 * @returns HTML string with links
 */
export function linkifyMentions(text: string, options: LinkifyOptions = {}): string {
	const {
		mentionBaseUrl = '/users/',
		hashtagBaseUrl = '/tags/',
		mentionClass = 'mention',
		hashtagClass = 'hashtag',
		urlClass = 'url',
		openInNewTab = true,
		maxUrlLength = 30,
		nofollow = true,
	} = options;

	let result = escapeHtml(text);
	const targetAttr = openInNewTab ? ' target="_blank"' : '';
	const relValue =
		`${openInNewTab ? 'noopener noreferrer' : ''}${nofollow ? ' nofollow' : ''}`.trim();
	const relAttr = relValue ? ` rel="${relValue}"` : '';

	// Process URLs first (to avoid linkifying URLs within other patterns)
	result = result.replace(PATTERNS.url, (match, url) => {
		const href = url.startsWith('http') ? url : `https://${url}`;
		try {
			new URL(href); // Validate URL
			const displayText = truncateUrl(href, maxUrlLength);
			return ` <a href="${href}" class="${urlClass}"${targetAttr}${relAttr}>${displayText}</a>`;
		} catch {
			return match; // Invalid URL, return as-is
		}
	});

	// Process mentions
	result = result.replace(PATTERNS.mention, (_match, mention) => {
		const username = mention.substring(1); // Remove @
		const href = mentionBaseUrl + username;
		return ` <a href="${href}" class="${mentionClass}"${targetAttr}${relAttr}>${mention}</a>`;
	});

	// Process hashtags
	result = result.replace(PATTERNS.hashtag, (_match, hashtag) => {
		const tag = hashtag.substring(1); // Remove #
		const href = hashtagBaseUrl + encodeURIComponent(tag);
		return ` <a href="${href}" class="${hashtagClass}"${targetAttr}${relAttr}>${hashtag}</a>`;
	});

	return result.trim();
}

/**
 * Extract mentions from text
 * @param text - The text to extract mentions from
 * @returns Array of mentions (without @ prefix)
 */
export function extractMentions(text: string): string[] {
	const mentions: string[] = [];

	PATTERNS.mention.lastIndex = 0;
	let match = PATTERNS.mention.exec(text);
	while (match !== null) {
		if (match[1]) {
			mentions.push(match[1].substring(1)); // Remove @ prefix
		}
		match = PATTERNS.mention.exec(text);
	}

	return [...new Set(mentions)]; // Remove duplicates
}

/**
 * Extract hashtags from text
 * @param text - The text to extract hashtags from
 * @returns Array of hashtags (without # prefix)
 */
export function extractHashtags(text: string): string[] {
	const hashtags: string[] = [];

	PATTERNS.hashtag.lastIndex = 0;
	let match = PATTERNS.hashtag.exec(text);
	while (match !== null) {
		if (match[1]) {
			hashtags.push(match[1].substring(1)); // Remove # prefix
		}
		match = PATTERNS.hashtag.exec(text);
	}

	return [...new Set(hashtags)]; // Remove duplicates
}

/**
 * Extract URLs from text
 * @param text - The text to extract URLs from
 * @returns Array of URLs
 */
export function extractUrls(text: string): string[] {
	const urls: string[] = [];

	PATTERNS.url.lastIndex = 0;
	let match = PATTERNS.url.exec(text);
	while (match !== null) {
		const url = match[1];
		if (url) {
			const href = url.startsWith('http') ? url : `https://${url}`;
			try {
				new URL(href); // Validate URL
				urls.push(href);
			} catch {
				// Invalid URL, skip
			}
		}
		match = PATTERNS.url.exec(text);
	}

	return [...new Set(urls)]; // Remove duplicates
}

/**
 * Check if text contains mentions
 */
export function hasMentions(text: string): boolean {
	PATTERNS.mention.lastIndex = 0;
	return PATTERNS.mention.test(text);
}

/**
 * Check if text contains hashtags
 */
export function hasHashtags(text: string): boolean {
	PATTERNS.hashtag.lastIndex = 0;
	return PATTERNS.hashtag.test(text);
}

/**
 * Check if text contains URLs
 */
export function hasUrls(text: string): boolean {
	PATTERNS.url.lastIndex = 0;
	return PATTERNS.url.test(text);
}
