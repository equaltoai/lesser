import type { Snippet } from 'svelte';
import { type ColorScheme } from '../stores/preferences';
interface Props {
	variant?: 'compact' | 'full';
	showPreview?: boolean;
	/** @deprecated showAdvanced is deprecated and will be removed in a future version. Custom colors require external CSS for CSP compliance. */
	showAdvanced?: boolean;
	/** @deprecated showWorkbench is deprecated and will be removed in a future version. Use the standalone ThemeWorkbench component for development purposes only. */
	showWorkbench?: boolean;
	class?: string;
	value?: ColorScheme;
	onThemeChange?: (theme: ColorScheme) => void;
	children?: Snippet;
}
declare const ThemeSwitcher: import('svelte').Component<Props, {}, ''>;
type ThemeSwitcher = ReturnType<typeof ThemeSwitcher>;
export default ThemeSwitcher;
//# sourceMappingURL=ThemeSwitcher.svelte.d.ts.map
