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
export declare function exportToMarkdown(options?: ExportMarkdownOptions): Promise<string>;
/**
 * Exports a chat conversation to Markdown.
 */
export declare function exportChatToMarkdown(options: ChatExportOptions): Promise<string>;
/**
 * Triggers a download of the markdown content.
 */
export declare function downloadMarkdown(markdown: string, filename: string): void;
//# sourceMappingURL=export-markdown.d.ts.map
