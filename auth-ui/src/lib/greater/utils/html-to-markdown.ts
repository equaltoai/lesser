import { unified } from 'unified';
import rehypeParse from 'rehype-parse';
import { toMdast } from 'hast-util-to-mdast';
import { toMarkdown } from 'mdast-util-to-markdown';
import { gfmToMarkdown } from 'mdast-util-gfm';

type HastChild = {
	tagName?: string;
	children?: HastChild[];
};

type HastRoot = {
	children: HastChild[];
};

/**
 * Converts HTML string to Markdown.
 * Uses GitHub Flavored Markdown (GFM) and secure defaults.
 * Fully ESM-compatible implementation using the unified ecosystem.
 *
 * @param html - The HTML string to convert
 * @returns The generated Markdown string
 */
export function htmlToMarkdown(html: string): string {
	if (!html || html.trim() === '') {
		return '';
	}

	// Parse HTML to HAST (HTML Abstract Syntax Tree)
	const processor = unified().use(rehypeParse, { fragment: true });

	const hastTree = processor.parse(html) as HastRoot;

	// Remove dangerous elements from the HAST tree
	removeDangerousElements(hastTree);

	// Convert HAST to MDAST (Markdown Abstract Syntax Tree)
	const mdastTree = toMdast(hastTree as unknown as Parameters<typeof toMdast>[0]);

	// Serialize MDAST to Markdown string with GFM support
	const markdown = toMarkdown(mdastTree as unknown as Parameters<typeof toMarkdown>[0], {
		extensions: [gfmToMarkdown()],
		bullet: '-',
		emphasis: '*',
		fences: true,
		listItemIndent: 'one',
	});

	return markdown.trim();
}

/**
 * Recursively removes dangerous elements (script, style, iframe, etc.) from a HAST tree.
 */
function removeDangerousElements(node: HastRoot | HastChild): void {
	if (!Array.isArray(node.children)) {
		return;
	}

	const dangerousTags = new Set([
		'script',
		'style',
		'iframe',
		'object',
		'embed',
		'link',
		'noscript',
		'form',
	]);

	node.children = node.children.filter((child) => {
		if (typeof child.tagName === 'string' && dangerousTags.has(child.tagName)) return false;
		return true;
	});

	// Recursively process remaining children
	for (const child of node.children) {
		if (Array.isArray(child.children)) {
			removeDangerousElements(child);
		}
	}
}
