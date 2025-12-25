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

@example Custom spacing value
```svelte
<Section spacing="8rem">
  <h2>Custom Spaced Section</h2>
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
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	/**
	 * Preset spacing values.
	 * @public
	 */
	type SpacingPreset = 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl';

	/**
	 * Background variant presets.
	 * @public
	 */
	type BackgroundVariant = 'default' | 'muted' | 'accent' | 'gradient';

	/**
	 * Gradient direction options.
	 * @public
	 */
	type GradientDirection =
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
		 * Can be a preset value or a custom CSS value (e.g., '8rem', '128px').
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
		 * @defaultValue 'md'
		 * @public
		 */
		spacing?: SpacingPreset | string | number;

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
		 * Background variant or custom CSS background value.
		 *
		 * Preset variants:
		 * - `default`: Transparent/inherit
		 * - `muted`: Subtle secondary background
		 * - `accent`: Primary color tinted background
		 * - `gradient`: Gradient background (use with gradientDirection)
		 *
		 * Can also accept custom CSS values like 'linear-gradient(...)' or '#f5f5f5'.
		 *
		 * @defaultValue 'default'
		 * @public
		 */
		background?: BackgroundVariant | string;

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

	// Preset spacing values for validation
	const SPACING_PRESETS: SpacingPreset[] = ['none', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl'];
	const BACKGROUND_PRESETS: BackgroundVariant[] = ['default', 'muted', 'accent', 'gradient'];

	let {
		spacing = 'md',
		padding = false,
		centered = false,
		background = 'default',
		gradientDirection = 'to-bottom',
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	// Check if spacing is a preset or custom value
	const isSpacingPreset = $derived(
		typeof spacing === 'string' && SPACING_PRESETS.includes(spacing as SpacingPreset)
	);

	// Check if background is a preset or custom value
	const isBackgroundPreset = $derived(
		typeof background === 'string' && BACKGROUND_PRESETS.includes(background as BackgroundVariant)
	);

	// Compute custom spacing style
	const customSpacingStyle = $derived.by(() => {
		if (isSpacingPreset) return '';
		if (typeof spacing === 'number') {
			return `--gr-section-custom-spacing: ${spacing}px;`;
		}
		return `--gr-section-custom-spacing: ${spacing};`;
	});

	// Compute custom background style
	const customBackgroundStyle = $derived.by(() => {
		if (isBackgroundPreset) return '';
		return `--gr-section-custom-background: ${background};`;
	});

	// Combine custom styles
	const customStyle = $derived.by(() => {
		const styles = [customSpacingStyle, customBackgroundStyle].filter(Boolean);
		return styles.length > 0 ? styles.join(' ') : undefined;
	});

	// Compute section classes
	const sectionClass = $derived(() => {
		let paddingClass = '';
		if (typeof padding === 'string') {
			paddingClass = `gr-section--padded-${padding}`;
		} else if (padding === true) {
			paddingClass = 'gr-section--padded-md';
		}

		const classes = ['gr-section'];

		// Spacing classes
		if (isSpacingPreset) {
			classes.push(`gr-section--spacing-${spacing}`);
		} else {
			classes.push('gr-section--spacing-custom');
		}

		// Background classes
		if (isBackgroundPreset && background !== 'default') {
			classes.push(`gr-section--bg-${background}`);
			if (background === 'gradient') {
				classes.push(`gr-section--gradient-${gradientDirection}`);
			}
		} else if (!isBackgroundPreset) {
			classes.push('gr-section--bg-custom');
		}

		if (paddingClass) classes.push(paddingClass);
		if (centered) classes.push('gr-section--centered');
		if (className) classes.push(className);

		return classes.filter(Boolean).join(' ');
	});
</script>

<section class={sectionClass()} style={customStyle} {...restProps}>
	{#if children}
		{@render children()}
	{/if}
</section>
