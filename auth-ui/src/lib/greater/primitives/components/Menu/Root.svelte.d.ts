import type { Snippet } from 'svelte';
import { type MenuPlacement } from './context';
interface Props {
	/** Whether the menu is open (bindable) */
	open?: boolean;
	/** Preferred placement of the menu content */
	placement?: MenuPlacement;
	/** Offset preset between trigger and content */
	offset?: 'sm' | 'md' | 'lg';
	/** Close menu when item is selected */
	closeOnSelect?: boolean;
	/** Enable keyboard navigation loop */
	loop?: boolean;
	/** Custom CSS class */
	class?: string;
	/** Child content (compound components) */
	children: Snippet;
	/** Called when open state changes */
	onOpenChange?: (open: boolean) => void;
}
declare const Root: import('svelte').Component<Props, {}, 'open'>;
type Root = ReturnType<typeof Root>;
export default Root;
//# sourceMappingURL=Root.svelte.d.ts.map
