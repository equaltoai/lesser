import { htmlToMarkdown } from './html-to-markdown.js';

export interface ExportMarkdownOptions {
	/** CSS selector for the element to export. Ignored if element is provided. */
	selector?: string;
	/** HTMLElement to export. Takes precedence over selector. */
	element?: HTMLElement;
	/** Title of the document, added as H1 or frontmatter title. */
	title?: string;
	/** Whether to include YAML frontmatter. */
	includeMetadata?: boolean;
	/** Whether to trigger a download of the generated file. */
	download?: boolean;
	/** Filename for the download. Defaults to 'export.md'. */
	filename?: string;
}

export interface ChatMessage {
	id: string;
	role: string;
	content: string;
	timestamp?: Date | string;
	name?: string;
}

export interface ChatExportOptions {
	messages: ChatMessage[];
	title?: string;
	date?: Date;
	includeMetadata?: boolean;
	download?: boolean;
	filename?: string;
}

/**
 * Exports a DOM element's content to Markdown.
 */
export async function exportToMarkdown(options: ExportMarkdownOptions = {}): Promise<string> {
	let element = options.element;
	if (!element && options.selector) {
		const el = document.querySelector(options.selector);
		if (el instanceof HTMLElement) {
			element = el;
		}
	}

	if (!element) {
		throw new Error('Element to export not found');
	}

	let markdown = htmlToMarkdown(element.innerHTML);

	// Prepend title if not in metadata
	if (options.title && !options.includeMetadata) {
		markdown = `# ${options.title}\n\n${markdown}`;
	}

	if (options.includeMetadata) {
		const date = new Date().toISOString();
		const fm = [
			'---',
			`title: ${options.title || 'Export'}`,
			`date: ${date}`,
			'type: export',
			'---',
			'',
			'',
		].join('\n');
		markdown = fm + markdown;
	}

	if (options.download) {
		downloadMarkdown(markdown, options.filename || 'export.md');
	}

	return markdown;
}

/**
 * Exports a chat conversation to Markdown.
 */
export async function exportChatToMarkdown(options: ChatExportOptions): Promise<string> {
	const {
		messages,
		title = 'Chat Export',
		date = new Date(),
		includeMetadata = true,
		download = false,
		filename = 'chat-export.md',
	} = options;

	let markdown = '';

	if (includeMetadata) {
		markdown += [
			'---',
			`title: ${title}`,
			`date: ${date.toISOString()}`,
			'type: chat-export',
			'---',
			'',
			'',
		].join('\n');
	}

	markdown += `# ${title}\n\n`;
	markdown += `*Exported on ${date.toLocaleDateString()}*\n\n---\n\n`;

	for (const msg of messages) {
		const roleName = msg.name || capitalize(msg.role);
		const time = msg.timestamp ? formatTime(msg.timestamp) : '';

		if (time) {
			markdown += `### ${time}\n\n`;
		}

		markdown += `**${roleName}:**\n`;

		// Ensure content spacing
		const content = msg.content.trim();
		markdown += `${content}\n\n`;

		// Separator if needed, or just spacing
		// markdown += `---\n\n`;
	}

	if (download) {
		downloadMarkdown(markdown, filename);
	}

	return markdown;
}

/**
 * Triggers a download of the markdown content.
 */
export function downloadMarkdown(markdown: string, filename: string): void {
	const blob = new Blob([markdown], { type: 'text/markdown;charset=utf-8' });
	const url = URL.createObjectURL(blob);

	const link = document.createElement('a');
	link.href = url;
	link.download = sanitizeFilename(filename);
	link.hidden = true;

	document.body.appendChild(link);
	link.click();

	document.body.removeChild(link);
	setTimeout(() => URL.revokeObjectURL(url), 100);
}

// Helpers

function capitalize(s: string): string {
	return s.charAt(0).toUpperCase() + s.slice(1);
}

function formatTime(dateOrString: Date | string): string {
	const d = typeof dateOrString === 'string' ? new Date(dateOrString) : dateOrString;
	if (isNaN(d.getTime())) return '';
	return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function sanitizeFilename(filename: string): string {
	// Remove potentially dangerous characters
	return filename.replace(/[^a-zA-Z0-9._-]/g, '_');
}
