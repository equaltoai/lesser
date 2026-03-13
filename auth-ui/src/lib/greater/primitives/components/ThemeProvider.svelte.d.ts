import type { Snippet } from 'svelte';
import { type ColorScheme } from '../stores/preferences';
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
declare const ThemeProvider: import('svelte').Component<Props, {}, ''>;
type ThemeProvider = ReturnType<typeof ThemeProvider>;
export default ThemeProvider;
//# sourceMappingURL=ThemeProvider.svelte.d.ts.map
