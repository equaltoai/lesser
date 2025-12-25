<!--
MarkdownRenderer component - Renders markdown content safely with syntax highlighting support.
Uses the unified ecosystem (remark + rehype) for ESM-compatible markdown processing.

@component
@example
```svelte
<MarkdownRenderer content="# Hello\n\nThis is **markdown**." />
```
-->
<script lang="ts">
	import { unified } from 'unified';
	import remarkParse from 'remark-parse';
	import remarkGfm from 'remark-gfm';
	import remarkRehype from 'remark-rehype';
	import rehypeSanitize, { defaultSchema } from 'rehype-sanitize';
	import rehypeStringify from 'rehype-stringify';
	import type { Schema } from 'hast-util-sanitize';

	interface Props {
		/**
		 * Markdown content to render.
		 */
		content: string;

		/**
		 * Whether to sanitize the HTML output.
		 * @default true
		 */
		sanitize?: boolean;

		/**
		 * List of allowed HTML tags. If not provided, uses default safe tags.
		 */
		allowedTags?: string[];

		/**
		 * Enable syntax highlighting for code blocks (uses styling class).
		 * @default true
		 */
		enableCodeHighlight?: boolean;

		/**
		 * Enable clickable links.
		 * @default true
		 */
		enableLinks?: boolean;

		/**
		 * Open links in new tab.
		 * @default true
		 */
		openLinksInNewTab?: boolean;

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Callback when rendering is complete.
		 */
		onRenderComplete?: () => void;

		/**
		 * Callback on error.
		 */
		onError?: (error: Error) => void;
	}

	let {
		content,
		sanitize = true,
		allowedTags,
		// enableCodeHighlight = true, // Handled by CSS
		enableLinks = true,
		openLinksInNewTab = true,
		class: className = '',
		onRenderComplete,
		onError,
		...restProps
	}: Props = $props();

	// Build custom sanitization schema
	function buildSanitizeSchema(): Schema {
		const schema: Schema = {
			...defaultSchema,
			tagNames: allowedTags || [
				'p',
				'br',
				'strong',
				'b',
				'em',
				'i',
				'code',
				'pre',
				'h1',
				'h2',
				'h3',
				'h4',
				'h5',
				'h6',
				'ul',
				'ol',
				'li',
				'a',
				'blockquote',
				'table',
				'thead',
				'tbody',
				'tr',
				'th',
				'td',
				'del',
				'img',
				'hr',
				'span',
				'div',
			],
			attributes: {
				...defaultSchema.attributes,
				a: enableLinks
					? ['href', 'title', ...(openLinksInNewTab ? (['target', 'rel'] as const) : [])]
					: [],
				img: ['src', 'alt', 'title'],
				'*': ['className', 'class'],
			},
		};

		return schema;
	}

	// Create the markdown processor
	function createProcessor() {
		const processor = unified().use(remarkParse).use(remarkGfm).use(remarkRehype, {
			allowDangerousHtml: false,
		});

		if (sanitize) {
			processor.use(rehypeSanitize, buildSanitizeSchema());
		}

		processor.use(rehypeStringify);

		return processor;
	}

	// Post-process HTML to add target="_blank" to links if needed
	function postProcessHtml(html: string): string {
		if (!enableLinks) {
			// Remove link hrefs but keep text
			return html.replace(/<a[^>]*>([^<]*)<\/a>/g, '$1');
		}

		if (openLinksInNewTab) {
			// Add target="_blank" and rel="noopener noreferrer" to all links
			return html.replace(
				/<a\s+href="([^"]*)"([^>]*)>/g,
				'<a href="$1" target="_blank" rel="noopener noreferrer"$2>'
			);
		}

		return html;
	}

	const renderedHtml = $derived.by(() => {
		try {
			if (!content) return '';

			const processor = createProcessor();
			const result = processor.processSync(content);
			let html = String(result);

			// Apply post-processing for link behavior
			html = postProcessHtml(html);

			return html;
		} catch (error: unknown) {
			if (onError && error instanceof Error) onError(error);
			// Fallback to escaped text
			return content
				.replace(/&/g, '&amp;')
				.replace(/</g, '&lt;')
				.replace(/>/g, '&gt;')
				.replace(/"/g, '&quot;')
				.replace(/'/g, '&#039;');
		}
	});

	$effect(() => {
		if (renderedHtml && onRenderComplete) {
			onRenderComplete();
		}
	});
</script>

<div class="gr-markdown {className}" {...restProps}>
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html renderedHtml}
</div>
