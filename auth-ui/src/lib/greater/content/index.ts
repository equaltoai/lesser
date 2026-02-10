/**
 * @fileoverview Greater Content - Rich content rendering components
 *
 * This package provides components for rendering rich content like code blocks
 * with syntax highlighting and markdown with sanitization. These components
 * have heavier dependencies (shiki, marked, isomorphic-dompurify) and are
 * separated from primitives to keep the core package lightweight.
 *
 * @version 3.0.0
 * @author Greater Contributors
 * @license AGPL-3.0-only
 * @public
 */

import type { ComponentProps } from 'svelte';
export type { ComponentProps } from 'svelte';

/**
 * CodeBlock component for syntax-highlighted code display.
 * @public
 */
export { default as CodeBlock } from './components/CodeBlock.svelte';

/**
 * MarkdownRenderer component for safe markdown rendering.
 * @public
 */
export { default as MarkdownRenderer } from './components/MarkdownRenderer.svelte';

// Component prop types
export type CodeBlockProps = ComponentProps<typeof import('./components/CodeBlock.svelte').default>;
export type MarkdownRendererProps = ComponentProps<
	typeof import('./components/MarkdownRenderer.svelte').default
>;
