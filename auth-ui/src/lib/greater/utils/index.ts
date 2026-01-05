/**
 * @fileoverview Greater Utils - Utility functions for web applications
 *
 * This package provides common utility functions for text processing,
 * time formatting, HTML sanitization, and keyboard interaction handling.
 * Designed for use in social media and content-rich applications.
 *
 * @version 1.0.0
 * @author Greater Contributors
 * @license AGPL-3.0-only
 * @public
 */

/**
 * HTML sanitization utilities for safe content rendering.
 * @public
 */
export * from './sanitizeHtml.js';

/**
 * Time formatting utilities for relative timestamps.
 * @public
 */
export * from './relativeTime.js';

/**
 * Link processing utilities for mentions and hashtags.
 * @public
 */
export * from './linkifyMentions.js';

/**
 * String formatting utilities.
 * @public
 */
export * from './truncateMiddle.js';

/**
 * Keyboard shortcut handling utilities.
 * @public
 */
export * from './keyboardShortcuts.js';

/**
 * Performance optimization utilities (debounce, throttle, caching, etc.).
 * @public
 */
export * from './performance.js';

/**
 * Clipboard utilities for copying text and elements.
 * @public
 */
export * from './clipboard.js';

/**
 * Markdown conversion and export utilities.
 * @public
 */
export { htmlToMarkdown } from './html-to-markdown.js';
export { exportToMarkdown, exportChatToMarkdown, downloadMarkdown } from './export-markdown.js';

export type { ExportMarkdownOptions, ChatExportOptions, ChatMessage } from './export-markdown.js';

/**
 * Preference store utilities.
 * @public
 */
export * from './preferences.js';

/**
 * Theme utilities.
 * @public
 */
export * from './theme/index.js';

/**
 * ID generation utilities.
 * @public
 */
export { default as IdProvider } from './id-provider.svelte';
export * from './use-stable-id.svelte.js';
