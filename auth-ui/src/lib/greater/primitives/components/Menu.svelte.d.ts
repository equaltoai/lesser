export interface MenuItemData {
	id: string;
	label: string;
	disabled?: boolean;
	submenu?: MenuItemData[];
	action?: () => void;
}

import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';

interface TriggerProps {
	open: boolean;
	toggle: () => void;
}

/**
 * Menu - A deprecated single-file Menu component.
 *
 * @deprecated Use the compound component pattern instead:
 * `Menu.Root`, `Menu.Trigger`, `Menu.Content`, `Menu.Item`.
 */
interface Props extends Omit<HTMLAttributes<HTMLUListElement>, 'role' | 'style'> {
	items: MenuItemData[];
	orientation?: 'horizontal' | 'vertical';
	class?: string;
	trigger?: Snippet<[TriggerProps]>;
	onItemSelect?: (item: MenuItemData) => void;
}

declare const Menu: import('svelte').Component<Props, {}, ''>;
type Menu = ReturnType<typeof Menu>;

export default Menu;
//# sourceMappingURL=Menu.svelte.d.ts.map
