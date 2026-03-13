import type { Snippet } from 'svelte';
interface Props {
	/** Custom CSS class */
	class?: string;
	/** Content children */
	children: Snippet;
	/** Minimum width matching trigger */
	matchTriggerWidth?: boolean;
	/** Maximum height before scrolling */
	maxHeight?: 'sm' | 'md' | 'lg';
	[key: string]: unknown;
}
declare const Content: import('svelte').Component<Props, {}, ''>;
type Content = ReturnType<typeof Content>;
export default Content;
//# sourceMappingURL=Content.svelte.d.ts.map
