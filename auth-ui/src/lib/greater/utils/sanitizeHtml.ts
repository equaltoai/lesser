import { unified } from 'unified';
import rehypeParse from 'rehype-parse';
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize';
import rehypeStringify from 'rehype-stringify';
import type { Schema } from 'hast-util-sanitize';

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
 * Default allowed tags for Fediverse content
 */
const DEFAULT_ALLOWED_TAGS = [
	'p',
	'br',
	'span',
	'a',
	'del',
	'pre',
	'code',
	'em',
	'strong',
	'b',
	'i',
	'u',
	's',
	'strike',
	'ul',
	'ol',
	'li',
	'blockquote',
	'h1',
	'h2',
	'h3',
	'h4',
	'h5',
	'h6',
];

/**
 * Default allowed attributes
 */
const DEFAULT_ALLOWED_ATTRIBUTES = ['href', 'title', 'class', 'rel', 'target'];

/**
 * Build a rehype-sanitize schema from SanitizeOptions
 */
function buildSchema(options: SanitizeOptions): Schema {
	const {
		allowedTags = DEFAULT_ALLOWED_TAGS,
		allowedAttributes = DEFAULT_ALLOWED_ATTRIBUTES,
		allowDataUri = false,
		addRelToExternalLinks = true,
		externalLinksInNewTab = true,
	} = options;

	// Build protocol list (no data: by default for security)
	const protocols: string[] = ['http', 'https', 'mailto'];
	if (allowDataUri) {
		protocols.push('data');
	}

	// Build allowed attributes map
	const attributeMap: Record<string, string[]> = {
		'*': allowedAttributes.filter((attr) => !['href', 'src'].includes(attr)),
	};

	// Add href to anchor tags if allowed
	if (allowedAttributes.includes('href')) {
		attributeMap['a'] = [
			'href',
			'title',
			...(addRelToExternalLinks ? ['rel'] : []),
			...(externalLinksInNewTab ? ['target'] : []),
		];
	}

	// Add src to img tags if allowed
	if (allowedAttributes.includes('src')) {
		attributeMap['img'] = ['src', 'alt', 'title'];
	}

	return {
		...defaultSchema,
		tagNames: allowedTags,
		attributes: attributeMap,
		protocols: {
			href: protocols,
			src: protocols,
		},
	};
}

/**
 * Sanitize HTML content using rehype-sanitize with an allow-list approach.
 * Fully ESM-compatible implementation.
 *
 * @param dirty - The potentially unsafe HTML string
 * @param options - Sanitization options
 * @returns Sanitized HTML string
 */
export function sanitizeHtml(dirty: string, options: SanitizeOptions = {}): string {
	// Handle null/undefined/empty input
	if (!dirty || typeof dirty !== 'string') {
		return '';
	}

	const schema = buildSchema(options);
	const { addRelToExternalLinks = true, externalLinksInNewTab = true } = options;

	const processor = unified()
		.use(rehypeParse, { fragment: true })
		.use(rehypeSanitize, schema)
		.use(rehypeStringify);

	let result = String(processor.processSync(dirty));

	// Post-process to add rel and target to external links
	if (addRelToExternalLinks || externalLinksInNewTab) {
		result = result.replace(/<a\s+href="(https?:\/\/[^"]+)"([^>]*)>/g, (_match, href, rest) => {
			let attributes = rest || '';

			if (addRelToExternalLinks && !attributes.includes('rel=')) {
				attributes += ' rel="noopener noreferrer"';
			}

			if (externalLinksInNewTab && !attributes.includes('target=')) {
				attributes += ' target="_blank"';
			}

			return `<a href="${href}"${attributes}>`;
		});
	}

	return result;
}

/**
 * Sanitize HTML for preview/excerpt (strips all HTML)
 * @param dirty - The HTML string to strip
 * @param maxLength - Maximum length of the output
 * @returns Plain text string
 */
export function sanitizeForPreview(dirty: string, maxLength = 200): string {
	// Handle null/undefined/empty input
	if (!dirty || typeof dirty !== 'string') {
		return '';
	}

	// Parse HTML and extract text only
	const processor = unified()
		.use(rehypeParse, { fragment: true })
		.use(rehypeSanitize, { tagNames: [], attributes: {} }) // Strip all tags
		.use(rehypeStringify);

	const textOnly = String(processor.processSync(dirty)).trim();

	if (textOnly.length <= maxLength) {
		return textOnly;
	}

	return textOnly.substring(0, maxLength).trim() + '...';
}
