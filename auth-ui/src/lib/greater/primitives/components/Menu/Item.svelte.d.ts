import type { Snippet } from 'svelte';
interface Props {
	/** Custom CSS class */
	class?: string;
	/** Item label text */
	label?: string;
	/** Item content (alternative to label) */
	children?: Snippet;
	/** Leading icon */
	icon?: Snippet;
	/** Keyboard shortcut display */
	shortcut?: string;
	/** Whether item is disabled */
	disabled?: boolean;
	/** Destructive/danger variant */
	destructive?: boolean;
	/** Click handler */
	onclick?: () => void;
}
declare const Item: import('svelte').Component<Props, {}, ''>;
type Item = ReturnType<typeof Item>;
export default Item;
//# sourceMappingURL=Item.svelte.d.ts.map
