<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Snippet } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';
	import { getMenuContext } from './context.svelte';

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

	let {
		class: className = '',
		label,
		children,
		icon,
		shortcut,
		disabled = false,
		destructive = false,
		onclick,
	}: Props = $props();

	const ctx = getMenuContext();
	const generatedId = useStableId('item');
	const itemId = $derived(`${ctx.menuId}-${generatedId.value}`);

	// Register item with context
	onMount(() => {
		ctx.registerItem({
			id: itemId,
			label,
			disabled,
			destructive,
			shortcut,
			onClick: onclick,
		});
	});

	onDestroy(() => {
		ctx.unregisterItem(itemId);
	});

	const isActive = $derived(ctx.items.findIndex((i) => i.id === itemId) === ctx.activeIndex);

	function handleClick(event: MouseEvent) {
		if (disabled) return;
		event.preventDefault();
		ctx.selectItem(itemId);
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (disabled) return;

		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			ctx.selectItem(itemId);
		}
		// Let other keys (ArrowDown, ArrowUp, Escape, etc.) bubble to Content's keydown handler
	}

	function handleMouseEnter() {
		if (disabled) return;
		const index = ctx.items.findIndex((i) => i.id === itemId);
		if (index !== -1) {
			ctx.setActiveIndex(index);
		}
	}
</script>

<div
	class="gr-menu-item {className}"
	class:gr-menu-item--active={isActive}
	class:gr-menu-item--disabled={disabled}
	class:gr-menu-item--destructive={destructive}
	role="menuitem"
	tabindex={disabled ? -1 : 0}
	aria-disabled={disabled || undefined}
	data-item-id={itemId}
	onclick={handleClick}
	onkeydown={handleKeyDown}
	onmouseenter={handleMouseEnter}
>
	{#if icon}
		<span class="gr-menu-item__icon">
			{@render icon()}
		</span>
	{/if}

	<span class="gr-menu-item__label">
		{#if children}
			{@render children()}
		{:else if label}
			{label}
		{/if}
	</span>

	{#if shortcut}
		<span class="gr-menu-item__shortcut">
			{shortcut}
		</span>
	{/if}
</div>

<style>
	.gr-menu-item {
		display: flex;
		align-items: center;
		gap: var(--gr-spacing-sm, 8px);
		padding: var(--gr-spacing-sm, 8px) var(--gr-spacing-md, 12px);
		border-radius: var(--gr-radius-sm, 4px);
		cursor: pointer;
		font-size: var(--gr-font-size-sm, 0.875rem);
		color: var(--gr-color-text, #111827);
		transition: background-color 0.15s ease;
		outline: none;
	}

	.gr-menu-item:hover,
	.gr-menu-item--active {
		background-color: var(--gr-color-surface-hover, #f3f4f6);
	}

	.gr-menu-item:focus-visible {
		box-shadow: 0 0 0 2px var(--gr-color-focus-ring, #3b82f6);
	}

	.gr-menu-item--disabled {
		cursor: not-allowed;
		opacity: 0.5;
	}

	.gr-menu-item--disabled:hover {
		background-color: transparent;
	}

	.gr-menu-item--destructive {
		color: var(--gr-color-error, #dc2626);
	}

	.gr-menu-item--destructive:hover,
	.gr-menu-item--destructive.gr-menu-item--active {
		background-color: var(--gr-color-error-surface, #fef2f2);
	}

	.gr-menu-item__icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
		flex-shrink: 0;
	}

	.gr-menu-item__label {
		flex: 1;
	}

	.gr-menu-item__shortcut {
		font-size: var(--gr-font-size-xs, 0.75rem);
		color: var(--gr-color-text-muted, #9ca3af);
		margin-left: auto;
	}
</style>
