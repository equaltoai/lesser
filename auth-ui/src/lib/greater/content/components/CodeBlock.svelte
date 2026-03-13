<!--
CodeBlock component - Syntax highlighting code block with copy button.

@component
@example
```svelte
<CodeBlock code="console.log('Hello')" language="javascript" showLineNumbers />
```
-->
<script lang="ts">
	import { CopyButton } from 'src/lib/greater/primitives';
	import type { Highlighter } from 'shiki';
	import type { HTMLAttributes } from 'svelte/elements';

	type MaxHeightPreset = 'sm' | 'md' | 'lg' | 'xl';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		/**
		 * The code to display.
		 */
		code: string;

		/**
		 * Language for syntax highlighting (e.g. 'javascript', 'python').
		 * If 'text' or undefined, no highlighting is performed.
		 */
		language?: string;

		/**
		 * Whether to show the copy button.
		 * @default true
		 */
		showCopy?: boolean;

		/**
		 * Whether to show line numbers.
		 * @default false
		 */
		showLineNumbers?: boolean;

		/**
		 * Lines to highlight (1-based index).
		 */
		highlightLines?: number[];

		/**
		 * Maximum height before scrolling.
		 */
		maxHeight?: MaxHeightPreset;

		/**
		 * Whether to wrap long lines.
		 * @default false
		 */
		wrap?: boolean;

		/**
		 * Filename to display in the header.
		 */
		filename?: string;

		/**
		 * Visual variant.
		 * @default 'outlined'
		 */
		variant?: 'outlined' | 'filled';

		/**
		 * Syntax theme.
		 * @default 'github-dark'
		 */
		theme?: string;

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Callback when code is copied.
		 */
		onCopy?: (code: string) => void;
	}

	let {
		code,
		language = 'text',
		showCopy = true,
		showLineNumbers = false,
		highlightLines = [],
		maxHeight,
		wrap = false,
		filename,
		variant = 'outlined',
		theme = 'github-dark',
		class: className = '',
		onCopy,
		style: _style,
		...restProps
	}: Props = $props();

	let highlightedCode = $state('');
	let loading = $state(true);

	// Static highlighter instance to share across components
	let highlighterPromise: Promise<Highlighter> | null = null;

	async function getHighlighterInstance() {
		if (!highlighterPromise) {
			// Lazy load shiki
			const { createHighlighter } = await import('shiki');
			highlighterPromise = createHighlighter({
				themes: ['github-dark', 'github-light'],
				langs: [
					'javascript',
					'typescript',
					'html',
					'css',
					'json',
					'bash',
					'python',
					'markdown',
					'yaml',
					'go',
					'rust',
					'svelte',
				],
			});
		}
		return highlighterPromise;
	}

	function escapeHtml(unsafe: string) {
		return unsafe
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#039;');
	}

	async function highlight() {
		if (!code) {
			highlightedCode = '';
			loading = false;
			return;
		}

		if (!language || language === 'text') {
			highlightedCode = escapeHtml(code);
			loading = false;
			return;
		}

		try {
			loading = true;
			const highlighter = await getHighlighterInstance();

			// Check if lang is loaded
			if (!highlighter.getLoadedLanguages().includes(language)) {
				highlightedCode = escapeHtml(code);
				loading = false;
				return;
			}

			highlightedCode = highlighter.codeToHtml(code, {
				lang: language,
				theme: theme,
				transformers: [
					{
						line(node, line) {
							if (showLineNumbers) {
								node.properties.class += ' line-number';
							}
							if (highlightLines.includes(line)) {
								node.properties.class += ' highlighted-line';
							}
						},
					},
				],
			});
			loading = false;
		} catch (e) {
			console.error('Syntax highlighting failed:', e);
			highlightedCode = escapeHtml(code);
			loading = false;
		}
	}

	$effect(() => {
		highlight();
	});

	function handleCopy() {
		onCopy?.(code);
	}
</script>

<div
	class="gr-code-block gr-code-block--{variant} {className}"
	class:gr-code-block--has-filename={!!filename}
	{...restProps}
>
	{#if filename}
		<div class="gr-code-block__header">
			<span class="gr-code-block__filename">{filename}</span>
			{#if showCopy}
				<CopyButton
					text={code}
					size="sm"
					variant="icon"
					buttonVariant="ghost"
					onCopy={handleCopy}
					class="gr-code-block__copy-btn"
				/>
			{/if}
		</div>
	{/if}

	<div
		class="gr-code-block__content"
		class:gr-code-block--max-height-sm={maxHeight === 'sm'}
		class:gr-code-block--max-height-md={maxHeight === 'md'}
		class:gr-code-block--max-height-lg={maxHeight === 'lg'}
		class:gr-code-block--max-height-xl={maxHeight === 'xl'}
		class:gr-code-block--wrap={wrap}
		class:gr-code-block--line-numbers={showLineNumbers}
	>
		{#if loading}
			<pre class="gr-code-block__pre"><code>{code}</code></pre>
		{:else}
			<!-- Shiki output is safe -->
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html highlightedCode}
		{/if}
	</div>

	{#if !filename && showCopy}
		<div class="gr-code-block__overlay">
			<CopyButton
				text={code}
				size="sm"
				variant="icon"
				buttonVariant="ghost"
				onCopy={handleCopy}
				class="gr-code-block__copy-btn"
			/>
		</div>
	{/if}
</div>
