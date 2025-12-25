import { type Component, type Snippet } from 'svelte';
interface Props {
	/**
	 * Icon to display for this item (overrides List icon).
	 */
	icon?: Component;
	/**
	 * Color of the icon (overrides List iconColor).
	 */
	iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
	/**
	 * Additional CSS classes.
	 */
	class?: string;
	/**
	 * Item content.
	 */
	children: Snippet;
}
/**
 * ListItem component - A single item within a List component.
 * Automatically inherits icon and styling from parent List.
 *
 *
 * @example
 * ```svelte
 * <ListItem>Feature description</ListItem>
 * <ListItem icon={CustomIcon}>Overridden icon</ListItem>
 * ```
 */
declare const ListItem: Component<Props, {}, ''>;
type ListItem = ReturnType<typeof ListItem>;
export default ListItem;
//# sourceMappingURL=ListItem.svelte.d.ts.map
