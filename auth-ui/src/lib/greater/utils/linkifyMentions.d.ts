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
 * Convert mentions, hashtags, and URLs in text to clickable links
 * @param text - The text to linkify
 * @param options - Linkify options
 * @returns HTML string with links
 */
export declare function linkifyMentions(text: string, options?: LinkifyOptions): string;
/**
 * Extract mentions from text
 * @param text - The text to extract mentions from
 * @returns Array of mentions (without @ prefix)
 */
export declare function extractMentions(text: string): string[];
/**
 * Extract hashtags from text
 * @param text - The text to extract hashtags from
 * @returns Array of hashtags (without # prefix)
 */
export declare function extractHashtags(text: string): string[];
/**
 * Extract URLs from text
 * @param text - The text to extract URLs from
 * @returns Array of URLs
 */
export declare function extractUrls(text: string): string[];
/**
 * Check if text contains mentions
 */
export declare function hasMentions(text: string): boolean;
/**
 * Check if text contains hashtags
 */
export declare function hasHashtags(text: string): boolean;
/**
 * Check if text contains URLs
 */
export declare function hasUrls(text: string): boolean;
//# sourceMappingURL=linkifyMentions.d.ts.map
