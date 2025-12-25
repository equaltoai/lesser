import type { Snippet } from 'svelte';
import { type ColorScheme } from '../stores/preferences';
import { type PalettePreset, type CustomPalette } from 'src/lib/greater/tokens';
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
declare const ThemeProvider: import('svelte').Component<Props, {}, ''>;
type ThemeProvider = ReturnType<typeof ThemeProvider>;
export default ThemeProvider;
//# sourceMappingURL=ThemeProvider.svelte.d.ts.map
