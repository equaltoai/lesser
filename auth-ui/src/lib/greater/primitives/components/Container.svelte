<!--
Container component - Max-width wrapper for content centering.

@component
@example
```svelte
<Container maxWidth="lg" padding="md" centered>
  <h1>Page Title</h1>
  <p>Page content...</p>
</Container>
```

@example Using size prop (alias for maxWidth)
```svelte
<Container size="xl" gutter="lg">
  <h1>Page Title</h1>
</Container>
```

@example Custom gutter
```svelte
<Container size="lg" gutter="2rem">
  <h1>Custom Padding</h1>
</Container>
```
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	/**
	 * Size/max-width preset values.
	 * @public
	 */
	type ContainerSize = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';

	/**
	 * Gutter/padding preset values.
	 * @public
	 */
	type GutterPreset = 'none' | 'sm' | 'md' | 'lg' | 'xl';

	/**
	 * Container component props interface.
	 *
	 * @public
	 */
	interface Props extends HTMLAttributes<HTMLDivElement> {
		/**
		 * Maximum width constraint.
		 * - `sm`: 640px
		 * - `md`: 768px
		 * - `lg`: 1024px (default)
		 * - `xl`: 1280px
		 * - `2xl`: 1536px
		 * - `full`: 100% (no constraint)
		 *
		 * @defaultValue 'lg'
		 * @public
		 */
		maxWidth?: ContainerSize;

		/**
		 * Alias for maxWidth. Provides consistent API with other components.
		 * If both size and maxWidth are provided, size takes precedence.
		 *
		 * @public
		 */
		size?: ContainerSize;

		/**
		 * Horizontal padding.
		 * - `false`: No padding
		 * - `true`: Default padding (1rem)
		 * - `sm`: 0.75rem
		 * - `md`: 1rem
		 * - `lg`: 1.5rem
		 *
		 * @defaultValue true
		 * @public
		 * @deprecated Use `gutter` instead for more explicit naming
		 */
		padding?: boolean | 'sm' | 'md' | 'lg';

		/**
		 * Horizontal padding (gutter) control.
		 * Can be a preset value or custom CSS value.
		 *
		 * Preset values:
		 * - `none`: No padding
		 * - `sm`: 0.75rem
		 * - `md`: 1rem (default)
		 * - `lg`: 1.5rem
		 * - `xl`: 2rem
		 *
		 * Can also accept custom CSS values like '2rem' or '24px'.
		 * Takes precedence over `padding` prop.
		 *
		 * @public
		 */
		gutter?: GutterPreset | string | number;

		/**
		 * Center content horizontally.
		 *
		 * @defaultValue true
		 * @public
		 */
		centered?: boolean;

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

	// Gutter preset values for validation
	const GUTTER_PRESETS: GutterPreset[] = ['none', 'sm', 'md', 'lg', 'xl'];

	let {
		maxWidth = 'lg',
		size,
		padding = true,
		gutter,
		centered = true,
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	// Resolve size (size prop takes precedence over maxWidth)
	const resolvedSize = $derived(size ?? maxWidth);

	// Check if gutter is a preset or custom value
	const isGutterPreset = $derived(
		typeof gutter === 'string' && GUTTER_PRESETS.includes(gutter as GutterPreset)
	);

	// Compute custom gutter style
	const customGutterStyle = $derived.by(() => {
		if (!gutter || isGutterPreset) return undefined;
		if (typeof gutter === 'number') {
			return `--gr-container-custom-gutter: ${gutter}px;`;
		}
		return `--gr-container-custom-gutter: ${gutter};`;
	});

	// Compute container classes
	const containerClass = $derived(() => {
		const classes = ['gr-container', `gr-container--max-${resolvedSize}`];

		// Handle gutter/padding
		if (gutter !== undefined) {
			// Gutter prop takes precedence
			if (isGutterPreset) {
				if (gutter !== 'none') {
					classes.push(`gr-container--padded-${gutter}`);
				}
			} else {
				classes.push('gr-container--padded-custom');
			}
		} else if (padding !== false) {
			// Fall back to padding prop
			if (typeof padding === 'string') {
				classes.push(`gr-container--padded-${padding}`);
			} else if (padding === true) {
				classes.push('gr-container--padded-md');
			}
		}

		if (centered) classes.push('gr-container--centered');
		if (className) classes.push(className);

		return classes.filter(Boolean).join(' ');
	});
</script>

<div class={containerClass()} style={customGutterStyle} {...restProps}>
	{#if children}
		{@render children()}
	{/if}
</div>
