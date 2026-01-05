<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import type { Snippet } from 'svelte';
	import { preferencesStore, type ColorScheme } from '../stores/preferences';
	import {
		palettes,
		generatePaletteCSS,
		getPresetGrayScale,
		type PalettePreset,
		type CustomPalette,
	} from 'src/lib/greater/tokens';

	interface Props {
		/** Color scheme: 'light', 'dark', 'high-contrast', or 'auto' */
		theme?: ColorScheme;
		/** Preset palette name: 'slate', 'stone', 'neutral', 'zinc', 'gray' */
		palette?: PalettePreset;
		/** Custom palette configuration with gray and/or primary color scales */
		customPalette?: CustomPalette;
		/** Custom heading font family (e.g., "'Crimson Pro', serif") */
		headingFont?: string;
		/** Custom body font family (e.g., "'Inter', sans-serif") */
		bodyFont?: string;
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
		customPalette,
		headingFont,
		bodyFont,
		enableSystemDetection = true,
		enablePersistence = true,
		preventFlash = true,
		children,
	}: Props = $props();

	// Generate custom CSS for palette and typography overrides
	let customCSS = $derived.by(() => {
		const cssRules: string[] = [];

		// Handle palette preset
		if (palette && palette in palettes) {
			const presetGrayScale = getPresetGrayScale(palette);
			if (presetGrayScale) {
				cssRules.push(generatePaletteCSS({ gray: presetGrayScale }));
			}
		}

		// Handle custom palette (takes precedence over preset)
		if (customPalette) {
			cssRules.push(generatePaletteCSS(customPalette));
		}

		// Handle typography customization
		if (headingFont) {
			cssRules.push(`--gr-typography-fontFamily-heading: ${headingFont};`);
		}
		if (bodyFont) {
			cssRules.push(`--gr-typography-fontFamily-sans: ${bodyFont};`);
		}

		return cssRules.length > 0 ? cssRules.join('\n') : '';
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

<div class="gr-theme-provider" data-theme-provider style={customCSS}>
	{@render children()}
</div>
