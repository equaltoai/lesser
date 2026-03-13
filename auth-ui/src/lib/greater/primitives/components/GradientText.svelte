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

	type GradientPreset = 'primary' | 'success' | 'warning' | 'error';
	type GradientDirection =
		| 'to-right'
		| 'to-left'
		| 'to-top'
		| 'to-bottom'
		| 'to-top-right'
		| 'to-top-left'
		| 'to-bottom-right'
		| 'to-bottom-left';

	interface Props {
		/**
		 * Gradient preset.
		 * - `primary`: Uses primary-600 to primary-400
		 * - `success`: Uses success-600 to success-400
		 * - `warning`: Uses warning-600 to warning-400
		 * - `error`: Uses error-600 to error-400
		 */
		gradient?: GradientPreset;

		/**
		 * Gradient direction preset.
		 */
		direction?: GradientDirection;

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
		direction = 'to-right',
		as: Tag = 'span',
		class: className = '',
		children,
		style: _style,
		...restProps
	}: Props & { style?: string } = $props();

	const gradientClass = $derived(() => {
		return [
			'gr-gradient-text',
			`gr-gradient-text--gradient-${gradient}`,
			`gr-gradient-text--direction-${direction}`,
			className,
		]
			.filter(Boolean)
			.join(' ');
	});
</script>

<svelte:element this={Tag} class={gradientClass()} {...restProps}>
	{@render children()}
</svelte:element>
