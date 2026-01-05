<script lang="ts">
	import type { Snippet } from 'svelte';
	import {
		createMenuContext,
		generateMenuId,
		MenuState,
		type MenuPlacement,
	} from './context.svelte';
	import { untrack } from 'svelte';
	import { createPositionObserver } from './positioning';

	interface Props {
		/** Whether the menu is open (bindable) */
		open?: boolean;
		/** Preferred placement of the menu content */
		placement?: MenuPlacement;
		/** Offset from trigger in pixels */
		offset?: number;
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

	let {
		open = $bindable(false),
		placement = 'bottom-start',
		offset = 4,
		closeOnSelect = true,
		loop = true,
		class: className = '',
		children,
		onOpenChange,
	}: Props = $props();

	// Generate unique IDs
	const menuId = generateMenuId();
	const triggerId = `${menuId}-trigger`;

	// Initialize state
	const menuState = new MenuState({
		menuId,
		triggerId,
		placement: untrack(() => placement),
		offset: untrack(() => offset),
		loop: untrack(() => loop),
		closeOnSelect: untrack(() => closeOnSelect),
		onOpenChange: (isOpen) => {
			open = isOpen;
			onOpenChange?.(isOpen);
		},
		initialOpen: untrack(() => open),
	});

	createMenuContext(menuState);

	// Sync external open prop and configuration
	$effect(() => {
		// Sync open state
		if (open !== menuState.isOpen) {
			if (open) menuState.open();
			else menuState.close();
		}

		// Sync configuration props
		if (placement) menuState.placement = placement;
		if (offset !== undefined) menuState.offset = offset;
		if (loop !== undefined) menuState.loop = loop;
		if (closeOnSelect !== undefined) menuState.closeOnSelect = closeOnSelect;

		// Update position if placement or offset changed while open
		if (menuState.isOpen && (placement || offset !== undefined)) {
			menuState.updatePosition();
		}
	});

	// Position observer
	let positionCleanup: (() => void) | null = null;
	$effect(() => {
		if (menuState.isOpen && menuState.triggerElement) {
			positionCleanup = createPositionObserver(menuState.triggerElement, menuState.updatePosition);
			menuState.updatePosition();
		}

		return () => {
			positionCleanup?.();
			positionCleanup = null;
		};
	});
</script>

<div class="gr-menu-root {className}">
	{@render children()}
</div>

<style>
	.gr-menu-root {
		position: relative;
		display: inline-block;
	}
</style>
