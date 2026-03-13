<!--
SimpleMenu - A convenience wrapper for the Menu compound component pattern.

Provides a simpler API for common use cases where you have an array of menu items
and want to render them automatically.

@component
@example
```svelte
<SimpleMenu items={menuItems} onItemSelect={handleSelect}>
  {#snippet trigger({ open, toggle })}
    <Button onclick={toggle}>Open Menu</Button>
  {/snippet}
</SimpleMenu>
```
-->
<script lang="ts" module>
	export interface MenuItem {
		id: string;
		label: string;
		disabled?: boolean;
		submenu?: MenuItem[];
	}
</script>

<script lang="ts">
	import * as Menu from './Menu/index';
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
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

	let { items, onItemSelect, trigger, placement = 'bottom-start', open = false }: Props = $props();

	let isOpen = $state(untrack(() => open));

	$effect(() => {
		isOpen = open;
	});

	function handleToggle() {
		isOpen = !isOpen;
	}

	function handleSelect(item: MenuItem) {
		onItemSelect?.(item);
		isOpen = false;
	}

	function handleOpenChange(newOpen: boolean) {
		isOpen = newOpen;
	}
</script>

<Menu.Root bind:open={isOpen} {placement} onOpenChange={handleOpenChange}>
	<Menu.Trigger>
		{@render trigger({ open: isOpen, toggle: handleToggle })}
	</Menu.Trigger>
	<Menu.Content>
		{#each items as item (item.id)}
			{#if item.submenu && item.submenu.length > 0}
				<Menu.Header>{item.label}</Menu.Header>
				{#each item.submenu as subItem (subItem.id)}
					<Menu.Item
						label={subItem.label}
						disabled={subItem.disabled}
						onclick={() => handleSelect(subItem)}
					/>
				{/each}
				<Menu.Separator />
			{:else}
				<Menu.Item label={item.label} disabled={item.disabled} onclick={() => handleSelect(item)} />
			{/if}
		{/each}
	</Menu.Content>
</Menu.Root>
