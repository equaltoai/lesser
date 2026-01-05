import type { Snippet } from 'svelte';
import { type ColorScheme } from '../stores/preferences';
interface Props {
	variant?: 'compact' | 'full';
	showPreview?: boolean;
	showAdvanced?: boolean;
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
