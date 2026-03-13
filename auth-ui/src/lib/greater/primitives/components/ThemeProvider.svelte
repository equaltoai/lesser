<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import type { Snippet } from 'svelte';
	import { preferencesStore, type ColorScheme } from '../stores/preferences';
	import type { PalettePreset, FontPreset } from 'src/lib/greater/tokens';

	interface Props {
		/** Color scheme: 'light', 'dark', 'high-contrast', or 'auto' */
		theme?: ColorScheme;
		/** Preset palette name: 'slate', 'stone', 'neutral', 'zinc', 'gray' */
		palette?: PalettePreset;
		/** Preset heading font: 'system', 'sans', 'serif', 'mono' */
		headingFontPreset?: FontPreset;
		/** Preset body font: 'system', 'sans', 'serif', 'mono' */
		bodyFontPreset?: FontPreset;
		/** Additional CSS class for custom theming via external CSS */
		class?: string;
		/** @deprecated Use app.html for flash prevention */
		enableSystemDetection?: boolean;
		/** @deprecated Use app.html for flash prevention */
		enablePersistence?: boolean;
		/** @deprecated Use app.html for flash prevention */
		preventFlash?: boolean;
		children: Snippet;
	}

	let {
		theme,
		palette,
		headingFontPreset,
		bodyFontPreset,
		class: className,
		enableSystemDetection = true,
		enablePersistence = true,
		preventFlash = true,
		children,
	}: Props = $props();

	// Generate class names based on preset values (CSP-compliant)
	let providerClasses = $derived.by(() => {
		const classes = ['gr-theme-provider'];

		// Add palette preset class
		if (palette) {
			classes.push(`gr-theme-provider--palette-${palette}`);
		}

		// Add typography preset classes
		if (headingFontPreset) {
			classes.push(`gr-theme-provider--heading-${headingFontPreset}`);
		}
		if (bodyFontPreset) {
			classes.push(`gr-theme-provider--body-${bodyFontPreset}`);
		}

		// Add custom class if provided
		if (className) {
			classes.push(className);
		}

		return classes.join(' ');
	});

	// Apply theme override if provided
	$effect(() => {
		if (theme && theme !== preferencesStore.preferences.colorScheme) {
			preferencesStore.setColorScheme(theme);
		}
	});

	// Initialize theme on mount
	onMount(() => {
		// The preferences store automatically initializes and applies theme
		// This is just to ensure it's initialized in case of SSR
		// If we have a theme prop, apply it
		if (theme) {
			preferencesStore.setColorScheme(theme);
		}

		// Return cleanup function
		return () => {
			// Cleanup is handled by the store
		};
	});

	// Note: preventFlash, enablePersistence, and enableSystemDetection props are kept
	// for API compatibility but flash prevention should be handled in app.html.
	// See Greater Components docs for the recommended approach.
	void untrack(() => preventFlash);
	void untrack(() => enablePersistence);
	void untrack(() => enableSystemDetection);
</script>

<!-- 
	Theme flash prevention should be handled in app.html for SvelteKit projects.
	The preventFlash prop is kept for API compatibility but the script injection
	via svelte:head doesn't work reliably. See Greater Components docs for 
	the recommended app.html approach.
-->

<div class={providerClasses} data-theme-provider>
	{@render children()}
</div>
