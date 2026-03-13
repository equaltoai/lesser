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

	type OffsetPreset = 'sm' | 'md' | 'lg';

	interface Props {
		/** Whether the menu is open (bindable) */
		open?: boolean;
		/** Preferred placement of the menu content */
		placement?: MenuPlacement;
		/** Offset preset between trigger and content */
		offset?: OffsetPreset;
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
		offset = 'sm',
		closeOnSelect = true,
		loop = true,
		class: className = '',
		children,
		onOpenChange,
	}: Props = $props();

	const offsetPx = $derived.by(() => {
		if (offset === 'lg') return 12;
		if (offset === 'md') return 8;
		return 4;
	});

	// Generate unique IDs
	const menuId = generateMenuId();
	const triggerId = `${menuId}-trigger`;

	// Initialize state
	const menuState = new MenuState({
		menuId,
		triggerId,
		placement: untrack(() => placement),
		offset: untrack(() => offsetPx),
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
		menuState.offset = offsetPx;
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

<div class="gr-menu-root gr-menu-root--offset-{offset} {className}">
	{@render children()}
</div>

<style>
	.gr-menu-root {
		position: relative;
		display: inline-block;
		--gr-menu-offset: 4px;
	}

	.gr-menu-root--offset-sm {
		--gr-menu-offset: 4px;
	}

	.gr-menu-root--offset-md {
		--gr-menu-offset: 8px;
	}

	.gr-menu-root--offset-lg {
		--gr-menu-offset: 12px;
	}
</style>
