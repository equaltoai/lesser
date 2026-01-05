export interface SanitizeOptions {
	/**
	 * Allow specific HTML tags
	 */
	allowedTags?: string[];
	/**
	 * Allow specific attributes
	 */
	allowedAttributes?: string[];
	/**
	 * Allow data URIs in src attributes
	 */
	allowDataUri?: boolean;
	/**
	 * Add rel="noopener noreferrer" to external links
	 */
	addRelToExternalLinks?: boolean;
	/**
	 * Open external links in new tab
	 */
	externalLinksInNewTab?: boolean;
}
/**
 * Sanitize HTML content using rehype-sanitize with an allow-list approach.
 * Fully ESM-compatible implementation.
 *
 * @param dirty - The potentially unsafe HTML string
 * @param options - Sanitization options
 * @returns Sanitized HTML string
 */
export declare function sanitizeHtml(dirty: string, options?: SanitizeOptions): string;
/**
 * Sanitize HTML for preview/excerpt (strips all HTML)
 * @param dirty - The HTML string to strip
 * @param maxLength - Maximum length of the output
 * @returns Plain text string
 */
export declare function sanitizeForPreview(dirty: string, maxLength?: number): string;
//# sourceMappingURL=sanitizeHtml.d.ts.map
