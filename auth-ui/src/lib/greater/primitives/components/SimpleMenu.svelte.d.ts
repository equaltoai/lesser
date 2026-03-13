export interface MenuItem {
	id: string;
	label: string;
	disabled?: boolean;
	submenu?: MenuItem[];
}
import type { Snippet } from 'svelte';
import type { MenuPlacement } from './Menu/context.svelte';
interface TriggerProps {
	open: boolean;
	toggle: () => void;
}
interface Props {
	/** Array of menu items to render */
	items: MenuItem[];
	/** Callback when an item is selected */
	onItemSelect?: (item: MenuItem) => void;
	/** Custom trigger snippet */
	trigger: Snippet<[TriggerProps]>;
	/** Menu placement */
	placement?: MenuPlacement;
	/** Whether menu starts open */
	open?: boolean;
}
/**
 * SimpleMenu - A convenience wrapper for the Menu compound component pattern.
 *
 * Provides a simpler API for common use cases where you have an array of menu items
 * and want to render them automatically.
 *
 *
 * @example
 * ```svelte
 * <SimpleMenu items={menuItems} onItemSelect={handleSelect}>
 * {#snippet trigger({ open, toggle })}
 *   <Button onclick={toggle}>Open Menu</Button>
 * {/snippet}
 * </SimpleMenu>
 * ```
 */
declare const SimpleMenu: import('svelte').Component<Props, {}, ''>;
type SimpleMenu = ReturnType<typeof SimpleMenu>;
export default SimpleMenu;
//# sourceMappingURL=SimpleMenu.svelte.d.ts.map
