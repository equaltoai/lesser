<!--
GradientText component - Eye-catching gradient text effect.

@component
@example
```svelte
<GradientText gradient="primary">Awesome Heading</GradientText>
<GradientText gradient="custom" from="#f00" to="#00f">Custom Gradient</GradientText>
```
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		/**
		 * Gradient preset or 'custom' mode.
		 * - `primary`: Uses primary-600 to primary-400
		 * - `success`: Uses success-600 to success-400
		 * - `warning`: Uses warning-600 to warning-400
		 * - `error`: Uses error-600 to error-400
		 * - `custom`: Uses from/to/via props
		 */
		gradient?: 'primary' | 'success' | 'warning' | 'error' | 'custom';

		/**
		 * Gradient direction (e.g., "to right", "45deg").
		 */
		direction?: string;

		/**
		 * Start color (for custom gradient).
		 */
		from?: string;

		/**
		 * End color (for custom gradient).
		 */
		to?: string;

		/**
		 * Optional middle color (for custom gradient).
		 */
		via?: string;

		/**
		 * Element tag to render.
		 */
		as?: string;

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Text content.
		 */
		children: Snippet;
	}

	let {
		gradient = 'primary',
		direction = 'to right',
		from,
		to,
		via,
		as: Tag = 'span',
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	const gradientStyle = $derived.by(() => {
		if (gradient === 'custom' && from && to) {
			const colors = via ? `${from}, ${via}, ${to}` : `${from}, ${to}`;
			return `linear-gradient(${direction}, ${colors})`;
		}

		// Preset gradients
		const colorMap: Record<string, string> = {
			primary: 'var(--gr-color-primary-600), var(--gr-color-primary-400)',
			success: 'var(--gr-color-success-600), var(--gr-color-success-400)',
			warning: 'var(--gr-color-warning-600), var(--gr-color-warning-400)',
			error: 'var(--gr-color-error-600), var(--gr-color-error-400)',
		};

		const colors = colorMap[gradient] || colorMap['primary'];
		return `linear-gradient(${direction}, ${colors})`;
	});
</script>

<svelte:element
	this={Tag}
	class="gr-gradient-text {className}"
	style:background-image={gradientStyle}
	{...restProps}
>
	{@render children()}
</svelte:element>
