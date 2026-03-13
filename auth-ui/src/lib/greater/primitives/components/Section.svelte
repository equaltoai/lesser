<!--
Section component - Semantic section wrapper with consistent vertical spacing.

@component
@example
```svelte
<Section spacing="lg" padding="md" centered>
  <h2>Section Title</h2>
  <p>Section content...</p>
</Section>
```

@example Extended spacing
```svelte
<Section spacing="3xl">
  <h2>Hero Section</h2>
</Section>
```

@example Background variants
```svelte
<Section background="muted">
  <h2>Muted Background</h2>
</Section>

<Section background="gradient" gradientDirection="to-bottom-right">
  <h2>Gradient Background</h2>
</Section>
```

@example Custom styling via external CSS
For custom spacing or background values beyond presets, use the `class` prop
with your own CSS classes defined in your application's stylesheet.
```svelte
<Section class="my-custom-section">
  <h2>Custom Section</h2>
</Section>
```
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	/**
	 * Preset spacing values.
	 * @public
	 */
	export type SpacingPreset = 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl';

	/**
	 * Background variant presets.
	 * @public
	 */
	export type BackgroundPreset = 'default' | 'muted' | 'accent' | 'gradient';

	/**
	 * Gradient direction options.
	 * @public
	 */
	export type GradientDirection =
		| 'to-top'
		| 'to-bottom'
		| 'to-left'
		| 'to-right'
		| 'to-top-left'
		| 'to-top-right'
		| 'to-bottom-left'
		| 'to-bottom-right';

	/**
	 * Section component props interface.
	 *
	 * @public
	 */
	interface Props extends HTMLAttributes<HTMLElement> {
		/**
		 * Vertical spacing (margin-top and margin-bottom).
		 * Must be a preset value for CSP compliance.
		 *
		 * Preset values:
		 * - `none`: No spacing
		 * - `sm`: 2rem
		 * - `md`: 4rem (default)
		 * - `lg`: 6rem
		 * - `xl`: 8rem
		 * - `2xl`: 10rem
		 * - `3xl`: 12rem
		 * - `4xl`: 16rem
		 *
		 * For custom spacing, use the `class` prop with external CSS.
		 *
		 * @defaultValue 'md'
		 * @public
		 */
		spacing?: SpacingPreset;

		/**
		 * Horizontal padding.
		 * - `false`: No padding
		 * - `true`: Default padding (1rem)
		 * - `sm`: 0.75rem
		 * - `md`: 1rem
		 * - `lg`: 1.5rem
		 *
		 * @defaultValue false
		 * @public
		 */
		padding?: boolean | 'sm' | 'md' | 'lg';

		/**
		 * Center content horizontally.
		 *
		 * @defaultValue false
		 * @public
		 */
		centered?: boolean;

		/**
		 * Background variant preset.
		 * Must be a preset value for CSP compliance.
		 *
		 * Preset variants:
		 * - `default`: Transparent/inherit
		 * - `muted`: Subtle secondary background
		 * - `accent`: Primary color tinted background
		 * - `gradient`: Gradient background (use with gradientDirection)
		 *
		 * For custom backgrounds, use the `class` prop with external CSS.
		 *
		 * @defaultValue 'default'
		 * @public
		 */
		background?: BackgroundPreset;

		/**
		 * Direction for gradient backgrounds.
		 * Only applies when background="gradient".
		 *
		 * @defaultValue 'to-bottom'
		 * @public
		 */
		gradientDirection?: GradientDirection;

		/**
		 * Additional CSS classes.
		 * Use this for custom styling beyond presets.
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
		spacing = 'md',
		padding = false,
		centered = false,
		background = 'default',
		gradientDirection = 'to-bottom',
		class: className = '',
		children,
		style: _style,
		...restProps
	}: Props = $props();

	// Compute section classes (CSP-compliant: no inline styles)
	const sectionClass = $derived(() => {
		const classes = ['gr-section'];

		// Spacing class (always a preset)
		classes.push(`gr-section--spacing-${spacing}`);

		// Background classes (only for non-default presets)
		if (background !== 'default') {
			classes.push(`gr-section--bg-${background}`);
			if (background === 'gradient') {
				classes.push(`gr-section--gradient-${gradientDirection}`);
			}
		}

		// Padding classes
		if (typeof padding === 'string') {
			classes.push(`gr-section--padded-${padding}`);
		} else if (padding === true) {
			classes.push('gr-section--padded-md');
		}

		// Centered class
		if (centered) {
			classes.push('gr-section--centered');
		}

		// Custom class
		if (className) {
			classes.push(className);
		}

		return classes.filter(Boolean).join(' ');
	});
</script>

<section class={sectionClass()} {...restProps}>
	{#if children}
		{@render children()}
	{/if}
</section>
