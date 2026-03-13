<!--
Text component - Paragraph and inline text component with size, weight, and color variants.

@component
@example
```svelte
<Text size="lg" color="secondary">Some text content</Text>
```
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	/**
	 * Text component props interface.
	 *
	 * @public
	 */
	interface Props extends HTMLAttributes<
		HTMLParagraphElement | HTMLSpanElement | HTMLDivElement | HTMLLabelElement
	> {
		/**
		 * HTML element to render.
		 * - `p`: Paragraph (default)
		 * - `span`: Inline span
		 * - `div`: Block div
		 * - `label`: Label element
		 *
		 * @defaultValue 'p'
		 * @public
		 */
		as?: 'p' | 'span' | 'div' | 'label';

		/**
		 * Text size.
		 * - `xs`: 0.75rem
		 * - `sm`: 0.875rem
		 * - `base`: 1rem (default)
		 * - `lg`: 1.125rem
		 * - `xl`: 1.25rem
		 * - `2xl`: 1.5rem
		 *
		 * @defaultValue 'base'
		 * @public
		 */
		size?: 'xs' | 'sm' | 'base' | 'lg' | 'xl' | '2xl';

		/**
		 * Font weight.
		 * - `normal`: 400 (default)
		 * - `medium`: 500
		 * - `semibold`: 600
		 * - `bold`: 700
		 *
		 * @defaultValue 'normal'
		 * @public
		 */
		weight?: 'normal' | 'medium' | 'semibold' | 'bold';

		/**
		 * Text color variant.
		 * - `primary`: Primary text color (default)
		 * - `secondary`: Muted text color
		 * - `tertiary`: Most muted text color
		 * - `success`: Success/positive color
		 * - `warning`: Warning color
		 * - `error`: Error/danger color
		 *
		 * @defaultValue 'primary'
		 * @public
		 */
		color?: 'primary' | 'secondary' | 'tertiary' | 'success' | 'warning' | 'error';

		/**
		 * Text alignment.
		 *
		 * @defaultValue 'left'
		 * @public
		 */
		align?: 'left' | 'center' | 'right' | 'justify';

		/**
		 * Whether text should truncate with ellipsis.
		 *
		 * @defaultValue false
		 * @public
		 */
		truncate?: boolean;

		/**
		 * Number of lines before truncating (requires truncate=true).
		 * If not set, single-line truncation is used.
		 *
		 * @public
		 */
		lines?: number;

		/**
		 * Additional CSS classes.
		 *
		 * @public
		 */
		class?: string;

		/**
		 * Content snippet.
		 *
		 * @public
		 */
		children?: Snippet;
	}

	let {
		as = 'p',
		size = 'base',
		weight = 'normal',
		color = 'primary',
		align = 'left',
		truncate = false,
		lines,
		class: className = '',
		children,
		style: _style,
		...restProps
	}: Props = $props();

	/**
	 * Generate clamp class for supported line counts (2-6).
	 * Returns empty string for unsupported values.
	 */
	function getClampClass(lines?: number): string {
		if (!lines) return '';
		if (lines >= 2 && lines <= 6) {
			return `gr-text--clamp-${lines}`;
		}
		// Out of range values require external CSS
		return '';
	}

	// Compute text classes
	const textClass = $derived(() => {
		const clampClass = truncate && lines ? getClampClass(lines) : '';
		const classes = [
			'gr-text',
			`gr-text--size-${size}`,
			`gr-text--weight-${weight}`,
			`gr-text--color-${color}`,
			`gr-text--align-${align}`,
			truncate && 'gr-text--truncate',
			clampClass,
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});
</script>

<svelte:element this={as} class={textClass()} {...restProps}>
	{#if children}
		{@render children()}
	{/if}
</svelte:element>
