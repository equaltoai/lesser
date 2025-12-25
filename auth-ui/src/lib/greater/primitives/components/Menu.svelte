<!--
	@deprecated This component is deprecated. Use the compound component pattern instead:
	
	<Menu.Root>
		<Menu.Trigger><Button>Options</Button></Menu.Trigger>
		<Menu.Content>
			<Menu.Item label="Edit" />
			<Menu.Item label="Delete" destructive />
		</Menu.Content>
	</Menu.Root>
	
	See packages/primitives/src/components/Menu/index.ts for the new API.
-->
<svelte:options
	customElement={{
		props: {
			items: {},
			orientation: {},
			class: {},
			trigger: {},
			onItemSelect: {},
		},
	}}
/>

<script lang="ts">
	import { tick, onMount } from 'svelte';

	onMount(() => {
		console.warn(
			'[Greater Components] Menu.svelte is deprecated. ' +
				'Please migrate to the compound component pattern: Menu.Root, Menu.Trigger, Menu.Content, Menu.Item. ' +
				'See documentation for migration guide.'
		);
	});
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	export interface MenuItemData {
		id: string;
		label: string;
		disabled?: boolean;
		submenu?: MenuItemData[];
		action?: () => void;
	}

	interface Props extends Omit<HTMLAttributes<HTMLUListElement>, 'role'> {
		items: MenuItemData[];
		orientation?: 'horizontal' | 'vertical';
		class?: string;
		trigger?: Snippet<[{ open: boolean; toggle: () => void }]>;
		onItemSelect?: (item: MenuItemData) => void;
	}

	let {
		items,
		orientation = 'vertical',
		class: className = '',
		trigger,
		onItemSelect,
		...restProps
	}: Props = $props<Props>();

	// State management
	let isOpen = $state(false);
	let activeIndex = $state(-1);
	let typeaheadString = $state('');
	let typeaheadTimeout: ReturnType<typeof setTimeout> | null = $state(null);
	let menuElement: HTMLUListElement | null = $state(null);
	let triggerContainer: HTMLElement | null = $state(null);
	let triggerFocusTarget: HTMLElement | null = $state(null);
	let expandedSubmenu = $state<string | null>(null);

	// Compute menu classes
	const menuClass = $derived(() => {
		const classes = ['gr-menu', `gr-menu--${orientation}`, className].filter(Boolean).join(' ');

		return classes;
	});

	// Get focusable items (non-disabled items)
	const focusableItems = $derived(() => {
		return items.filter((item) => !item.disabled);
	});

	const focusableSelector =
		'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

	function getTriggerFocusTarget() {
		if (!triggerContainer) return null;
		const focusable = triggerContainer.querySelector(focusableSelector) as HTMLElement | null;
		return focusable ?? triggerContainer;
	}

	function rememberTriggerFocus(event: FocusEvent) {
		const target = event.target as HTMLElement | null;
		if (target && triggerContainer?.contains(target)) {
			triggerFocusTarget = target;
		}
	}

	$effect(() => {
		if (!triggerContainer) return;

		triggerFocusTarget = getTriggerFocusTarget();

		const observer = new MutationObserver(() => {
			triggerFocusTarget = getTriggerFocusTarget();
		});

		observer.observe(triggerContainer, { childList: true, subtree: true });

		return () => observer.disconnect();
	});

	function toggle(event?: Event) {
		if (event?.currentTarget instanceof HTMLElement) {
			triggerFocusTarget = event.currentTarget;
		}
		isOpen = !isOpen;
		if (isOpen) {
			activeIndex = -1;
			// Focus first item when opened via keyboard
			requestAnimationFrame(() => {
				const firstItem = menuElement?.querySelector(
					'[role="menuitem"]:not([aria-disabled="true"])'
				) as HTMLElement;
				if (firstItem) {
					firstItem.focus();
					activeIndex = 0;
				}
			});
		}
	}

	async function close() {
		isOpen = false;
		activeIndex = -1;
		expandedSubmenu = null;
		await tick();
		const focusTarget = triggerFocusTarget ?? getTriggerFocusTarget();
		focusTarget?.focus();
	}

	function selectItem(item: MenuItemData) {
		if (item.disabled) return;

		if (item.submenu) {
			expandedSubmenu = expandedSubmenu === item.id ? null : item.id;
			return;
		}

		item.action?.();
		onItemSelect?.(item);
		close();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (!isOpen) return;

		switch (event.key) {
			case 'Escape':
				event.preventDefault();
				close();
				break;

			case 'ArrowDown':
				if (orientation === 'vertical') {
					event.preventDefault();
					moveToNext();
				}
				break;

			case 'ArrowUp':
				if (orientation === 'vertical') {
					event.preventDefault();
					moveToPrevious();
				}
				break;

			case 'ArrowRight': {
				if (orientation === 'horizontal') {
					event.preventDefault();
					moveToNext();
				} else {
					// Expand submenu in vertical orientation
					const currentItem = focusableItems()[activeIndex];
					if (currentItem?.submenu) {
						event.preventDefault();
						expandedSubmenu = currentItem.id;
					}
				}
				break;
			}

			case 'ArrowLeft': {
				if (orientation === 'horizontal') {
					event.preventDefault();
					moveToPrevious();
				} else {
					// Collapse submenu in vertical orientation
					event.preventDefault();
					expandedSubmenu = null;
				}
				break;
			}

			case 'Home':
				event.preventDefault();
				moveToFirst();
				break;

			case 'End':
				event.preventDefault();
				moveToLast();
				break;

			case 'Enter':
			case ' ': {
				event.preventDefault();
				const currentItem = focusableItems()[activeIndex];
				if (currentItem) {
					selectItem(currentItem);
				}
				break;
			}

			default:
				// Typeahead functionality
				if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
					handleTypeahead(event.key.toLowerCase());
				}
				break;
		}
	}

	function moveToNext() {
		const nextIndex = (activeIndex + 1) % focusableItems().length;
		setActiveIndex(nextIndex);
	}

	function moveToPrevious() {
		const prevIndex = activeIndex <= 0 ? focusableItems().length - 1 : activeIndex - 1;
		setActiveIndex(prevIndex);
	}

	function moveToFirst() {
		setActiveIndex(0);
	}

	function moveToLast() {
		setActiveIndex(focusableItems().length - 1);
	}

	function setActiveIndex(index: number) {
		activeIndex = index;
		const menuItem = menuElement?.querySelector(`[data-menu-index="${index}"]`) as HTMLElement;
		menuItem?.focus();
	}

	function handleTypeahead(key: string) {
		// Clear previous timeout
		if (typeaheadTimeout) {
			clearTimeout(typeaheadTimeout);
		}

		typeaheadString += key;

		// Find matching item
		const matchingIndex = focusableItems().findIndex((item) =>
			item.label.toLowerCase().startsWith(typeaheadString)
		);

		if (matchingIndex !== -1) {
			setActiveIndex(matchingIndex);
		}

		// Clear typeahead string after delay
		typeaheadTimeout = setTimeout(() => {
			typeaheadString = '';
			typeaheadTimeout = null;
		}, 1000);
	}

	function handleClickOutside(event: MouseEvent) {
		if (
			isOpen &&
			menuElement &&
			!menuElement.contains(event.target as Node) &&
			triggerContainer &&
			!triggerContainer.contains(event.target as Node)
		) {
			close();
		}
	}

	// Click outside handler
	$effect(() => {
		if (isOpen) {
			document.addEventListener('click', handleClickOutside);
			return () => document.removeEventListener('click', handleClickOutside);
		}
	});
</script>

<div class="gr-menu-container">
	{#if trigger}
		<div bind:this={triggerContainer} onfocusin={rememberTriggerFocus}>
			{@render trigger({ open: isOpen, toggle })}
		</div>
	{/if}

	{#if isOpen}
		<ul
			bind:this={menuElement}
			class={menuClass()}
			role={orientation === 'horizontal' ? 'menubar' : 'menu'}
			aria-orientation={orientation}
			onkeydown={handleKeydown}
			tabindex="-1"
			{...restProps}
		>
			{#each items as item (item.id)}
				{@const focusableIndex = focusableItems().findIndex((fi) => fi.id === item.id)}
				<li role="none" class="gr-menu__item-wrapper">
					<div
						role="menuitem"
						class="gr-menu__item"
						class:gr-menu__item--active={focusableIndex === activeIndex}
						class:gr-menu__item--disabled={item.disabled}
						class:gr-menu__item--has-submenu={!!item.submenu}
						data-menu-index={focusableIndex}
						tabindex={item.disabled ? -1 : focusableIndex === activeIndex ? 0 : -1}
						aria-disabled={item.disabled}
						aria-expanded={item.submenu ? expandedSubmenu === item.id : undefined}
						aria-haspopup={item.submenu ? 'menu' : undefined}
						onclick={() => selectItem(item)}
						onfocus={() => {
							if (!item.disabled) {
								activeIndex = focusableIndex;
							}
						}}
						onkeydown={(event) => {
							if ((event.key === 'Enter' || event.key === ' ') && !item.disabled) {
								event.preventDefault();
								event.stopPropagation();
								selectItem(item);
							}
						}}
					>
						<span class="gr-menu__item-label">{item.label}</span>
						{#if item.submenu}
							<span class="gr-menu__item-arrow" aria-hidden="true">
								<svg
									width="16"
									height="16"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
								>
									<polyline points="9 18 15 12 9 6"></polyline>
								</svg>
							</span>
						{/if}
					</div>

					{#if item.submenu && expandedSubmenu === item.id}
						<ul class="gr-menu gr-menu--submenu" role="menu" aria-orientation="vertical">
							{#each item.submenu as subItem (subItem.id)}
								<li role="none" class="gr-menu__item-wrapper">
									<div
										role="menuitem"
										class="gr-menu__item"
										class:gr-menu__item--disabled={subItem.disabled}
										tabindex={subItem.disabled ? -1 : 0}
										aria-disabled={subItem.disabled}
										onclick={() => selectItem(subItem)}
										onkeydown={(event) => {
											if ((event.key === 'Enter' || event.key === ' ') && !subItem.disabled) {
												event.preventDefault();
												event.stopPropagation();
												selectItem(subItem);
											}
										}}
									>
										<span class="gr-menu__item-label">{subItem.label}</span>
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
