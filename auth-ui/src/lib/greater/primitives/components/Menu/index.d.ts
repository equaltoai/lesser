/**
 * Menu Compound Component
 *
 * A dropdown menu component with compound component pattern for flexible composition.
 *
 * @example
 * ```svelte
 * <Menu.Root>
 *   <Menu.Trigger>
 *     <Button>Options</Button>
 *   </Menu.Trigger>
 *   <Menu.Content>
 *     <Menu.Header>User Actions</Menu.Header>
 *     <Menu.Item label="Edit" icon={EditIcon} shortcut="⌘E" />
 *     <Menu.Item label="Duplicate" shortcut="⌘D" />
 *     <Menu.Separator />
 *     <Menu.Item label="Delete" destructive onclick={handleDelete} />
 *   </Menu.Content>
 * </Menu.Root>
 * ```
 *
 * @module @equaltoai/greater-components/primitives/Menu
 */
export { default as Root } from './Root.svelte';
export { default as Trigger } from './Trigger.svelte';
export { default as Content } from './Content.svelte';
export { default as Header } from './Header.svelte';
export { default as Item } from './Item.svelte';
export { default as Separator } from './Separator.svelte';
export type { MenuPlacement, MenuItemConfig, MenuContextValue } from './context.svelte';
//# sourceMappingURL=index.d.ts.map
