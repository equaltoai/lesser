<script lang="ts">
	import { tick } from 'svelte';
	import type { Snippet } from 'svelte';
	import { getMenuContext } from './context.svelte';

	interface Props {
		/** Custom CSS class */
		class?: string;
		/** Content children */
		children: Snippet;
		/** Minimum width matching trigger */
		matchTriggerWidth?: boolean;
		/** Maximum height before scrolling */
		maxHeight?: string;
		[key: string]: unknown;
	}

	let {
		class: className = '',
		children,
		matchTriggerWidth = false,
		maxHeight = '300px',
		...restProps
	}: Props = $props();

	const ctx = getMenuContext();

	let contentRef: HTMLElement | null = $state(null);
	let typeaheadBuffer = $state('');
	let typeaheadTimeout: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		ctx.setContentElement(contentRef);
	});

	// Click outside handler
	$effect(() => {
		if (!ctx.isOpen) return;

		function handleClickOutside(event: MouseEvent) {
			const target = event.target as Node;
			if (
				contentRef &&
				!contentRef.contains(target) &&
				ctx.triggerElement &&
				!ctx.triggerElement.contains(target)
			) {
				ctx.close();
			}
		}

		// Delay to prevent immediate close on open click
		const timeoutId = setTimeout(() => {
			document.addEventListener('click', handleClickOutside);
		}, 0);

		return () => {
			clearTimeout(timeoutId);
			document.removeEventListener('click', handleClickOutside);
		};
	});

	// Focus first item when opened
	$effect(() => {
		if (ctx.isOpen && contentRef) {
			tick().then(() => {
				const firstItem = contentRef?.querySelector(
					'[role="menuitem"]:not([aria-disabled="true"])'
				) as HTMLElement;
				firstItem?.focus();
			});
		}
	});

	// Document-level Escape key handler - ensures Escape works from any focused element
	$effect(() => {
		if (!ctx.isOpen) return;

		function handleEscapeKey(event: KeyboardEvent) {
			if (event.key === 'Escape') {
				event.preventDefault();
				event.stopPropagation();
				ctx.close();
			}
		}

		document.addEventListener('keydown', handleEscapeKey, true); // Capture phase

		return () => {
			document.removeEventListener('keydown', handleEscapeKey, true);
		};
	});

	function handleKeyDown(event: KeyboardEvent) {
		const enabledItems = ctx.items.filter((item) => !item.disabled);
		const currentEnabledIndex = enabledItems.findIndex((_, i) => {
			const actualIndex = ctx.items.indexOf(enabledItems[i]!);
			return actualIndex === ctx.activeIndex;
		});

		if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) && enabledItems.length === 0) {
			return;
		}

		switch (event.key) {
			case 'ArrowDown': {
				event.preventDefault();
				const nextItem =
					currentEnabledIndex < enabledItems.length - 1
						? enabledItems[currentEnabledIndex + 1]
						: enabledItems[0];

				if (nextItem) {
					const nextIndex = ctx.items.indexOf(nextItem);
					ctx.setActiveIndex(nextIndex);
					focusItemAtIndex(nextIndex);
				}
				break;
			}
			case 'ArrowUp': {
				event.preventDefault();
				const prevItem =
					currentEnabledIndex > 0
						? enabledItems[currentEnabledIndex - 1]
						: enabledItems[enabledItems.length - 1];

				if (prevItem) {
					const prevIndex = ctx.items.indexOf(prevItem);
					ctx.setActiveIndex(prevIndex);
					focusItemAtIndex(prevIndex);
				}
				break;
			}
			case 'Home': {
				event.preventDefault();
				const firstItem = enabledItems[0];
				if (firstItem) {
					const firstIndex = ctx.items.indexOf(firstItem);
					ctx.setActiveIndex(firstIndex);
					focusItemAtIndex(firstIndex);
				}
				break;
			}
			case 'End': {
				event.preventDefault();
				const lastItem = enabledItems[enabledItems.length - 1];
				if (lastItem) {
					const lastIndex = ctx.items.indexOf(lastItem);
					ctx.setActiveIndex(lastIndex);
					focusItemAtIndex(lastIndex);
				}
				break;
			}
			case 'Escape':
				event.preventDefault();
				ctx.close();
				break;
			case 'Tab':
				event.preventDefault();
				ctx.close();
				break;
			case 'Enter':
			case ' ': {
				event.preventDefault();
				const activeItem = ctx.items[ctx.activeIndex];
				if (activeItem && !activeItem.disabled) {
					ctx.selectItem(activeItem.id);
				}
				break;
			}
			default:
				// Typeahead search
				if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
					handleTypeahead(event.key);
				}
				break;
		}
	}

	function handleTypeahead(char: string) {
		if (typeaheadTimeout) {
			clearTimeout(typeaheadTimeout);
		}

		typeaheadBuffer += char.toLowerCase();

		// Find matching item
		const matchIndex = ctx.items.findIndex(
			(item) => !item.disabled && item.label?.toLowerCase().startsWith(typeaheadBuffer)
		);

		if (matchIndex !== -1) {
			ctx.setActiveIndex(matchIndex);
			focusItemAtIndex(matchIndex);
		}

		typeaheadTimeout = setTimeout(() => {
			typeaheadBuffer = '';
			typeaheadTimeout = null;
		}, 500);
	}

	function focusItemAtIndex(index: number) {
		tick().then(() => {
			const items = contentRef?.querySelectorAll('[role="menuitem"]');
			const item = items?.[index] as HTMLElement;
			item?.focus();
		});
	}

	const triggerWidth = $derived(ctx.triggerElement?.getBoundingClientRect().width ?? 0);
</script>

{#if ctx.isOpen}
	<div
		bind:this={contentRef}
		class="gr-menu-content {className}"
		role="menu"
		id={ctx.menuId}
		aria-labelledby={ctx.triggerId}
		tabindex="-1"
		data-placement={ctx.position.placement}
		style:left="{ctx.position.x}px"
		style:top="{ctx.position.y}px"
		style:min-width={matchTriggerWidth ? `${triggerWidth}px` : undefined}
		style:max-height={maxHeight}
		onkeydown={handleKeyDown}
		{...restProps}
	>
		{@render children()}
	</div>
{/if}

<style>
	.gr-menu-content {
		position: fixed;
		z-index: 1000;
		background: var(--gr-color-surface, #ffffff);
		border: 1px solid var(--gr-color-border, #e5e7eb);
		border-radius: var(--gr-radius-md, 8px);
		box-shadow: var(--gr-shadow-lg, 0 10px 15px -3px rgba(0, 0, 0, 0.1));
		padding: var(--gr-spacing-xs, 4px);
		overflow-y: auto;
		outline: none;
	}

	.gr-menu-content[data-placement^='top'] {
		transform-origin: bottom;
	}

	.gr-menu-content[data-placement^='bottom'] {
		transform-origin: top;
	}
</style>
