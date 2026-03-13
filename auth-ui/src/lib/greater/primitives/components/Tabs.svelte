<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { untrack, type Snippet } from 'svelte';

	interface TabData {
		id: string;
		label: string;
		disabled?: boolean;
		content?: Snippet;
	}

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'role'> {
		tabs: TabData[];
		activeTab?: string;
		orientation?: 'horizontal' | 'vertical';
		activation?: 'automatic' | 'manual';
		variant?: 'default' | 'pills' | 'underline';
		class?: string;
		onTabChange?: (tabId: string) => void;
	}

	let {
		tabs = [],
		activeTab = undefined,
		orientation = 'horizontal',
		activation = 'automatic',
		variant = 'default',
		class: className = '',
		onTabChange,
		style: _style,
		...restProps
	}: Props = $props();

	// State management - initialize as undefined, sync via effect
	let currentActiveTab: string | undefined = $state(undefined);
	let focusedTabIndex = $state(0);
	let tablistElement: HTMLElement | null = $state(null);

	// Sync external activeTab prop with internal state
	$effect(() => {
		// Initialize on first run or when activeTab prop changes
		const targetTab = activeTab ?? tabs[0]?.id;
		if (targetTab !== undefined && targetTab !== untrack(() => currentActiveTab)) {
			currentActiveTab = targetTab;
			const tabIndex = tabs.findIndex((tab) => tab.id === targetTab);
			if (tabIndex !== -1) {
				focusedTabIndex = tabIndex;
			}
		}
	});

	// Compute container classes
	const containerClass = $derived(() => {
		const classes = ['gr-tabs', `gr-tabs--${orientation}`, `gr-tabs--${variant}`, className]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	function selectTab(tabId: string) {
		const tab = tabs.find((t) => t.id === tabId);
		if (tab && !tab.disabled && tabId !== currentActiveTab) {
			currentActiveTab = tabId;
			const tabIndex = tabs.findIndex((t) => t.id === tabId);
			focusedTabIndex = tabIndex;
			onTabChange?.(tabId);
		}
	}

	function focusTab(index: number) {
		focusedTabIndex = index;
		const tabButton = tablistElement?.querySelector(`[data-tab-index="${index}"]`) as HTMLElement;
		tabButton?.focus();

		// Automatic activation - select tab on focus
		if (activation === 'automatic') {
			const tab = tabs[index];
			if (tab && !tab.disabled) {
				selectTab(tab.id);
			}
		}
	}

	function handleTabKeydown(event: KeyboardEvent, tabIndex: number) {
		const tab = tabs[tabIndex];

		switch (event.key) {
			case 'ArrowRight':
				if (orientation === 'horizontal') {
					event.preventDefault();
					moveToNextTab();
				}
				break;

			case 'ArrowLeft':
				if (orientation === 'horizontal') {
					event.preventDefault();
					moveToPreviousTab();
				}
				break;

			case 'ArrowDown':
				if (orientation === 'vertical') {
					event.preventDefault();
					moveToNextTab();
				}
				break;

			case 'ArrowUp':
				if (orientation === 'vertical') {
					event.preventDefault();
					moveToPreviousTab();
				}
				break;

			case 'Home':
				event.preventDefault();
				moveToFirstTab();
				break;

			case 'End':
				event.preventDefault();
				moveToLastTab();
				break;

			case 'Enter':
			case ' ':
				if (activation === 'manual') {
					event.preventDefault();
					if (tab && !tab.disabled) {
						selectTab(tab.id);
					}
				}
				break;
		}
	}

	function moveToNextTab() {
		let nextIndex = focusedTabIndex + 1;

		// Skip disabled tabs
		while (nextIndex < tabs.length && tabs[nextIndex]?.disabled) {
			nextIndex++;
		}

		// Wrap around to first non-disabled tab
		if (nextIndex >= tabs.length) {
			nextIndex = tabs.findIndex((tab) => !tab.disabled);
		}

		if (nextIndex !== -1 && nextIndex !== focusedTabIndex) {
			focusTab(nextIndex);
		}
	}

	function moveToPreviousTab() {
		let prevIndex = focusedTabIndex - 1;

		// Skip disabled tabs
		while (prevIndex >= 0 && tabs[prevIndex]?.disabled) {
			prevIndex--;
		}

		// Wrap around to last non-disabled tab
		if (prevIndex < 0) {
			prevIndex = tabs.length - 1;
			while (prevIndex >= 0 && tabs[prevIndex]?.disabled) {
				prevIndex--;
			}
		}

		if (prevIndex !== -1 && prevIndex !== focusedTabIndex) {
			focusTab(prevIndex);
		}
	}

	function moveToFirstTab() {
		const firstIndex = tabs.findIndex((tab) => !tab.disabled);
		if (firstIndex !== -1 && firstIndex !== focusedTabIndex) {
			focusTab(firstIndex);
		}
	}

	function moveToLastTab() {
		let lastIndex = tabs.length - 1;
		while (lastIndex >= 0 && tabs[lastIndex]?.disabled) {
			lastIndex--;
		}
		if (lastIndex !== -1 && lastIndex !== focusedTabIndex) {
			focusTab(lastIndex);
		}
	}

	// Generate deterministic ID prefix for accessibility so SSR + CSR stay in sync.
	const tabsId = $derived(() => {
		const key = tabs.map((tab) => tab.id).join('-') || 'default';
		return `tabs-${key}`;
	});
</script>

<div class={containerClass()} {...restProps}>
	<div
		bind:this={tablistElement}
		class="gr-tabs__tablist"
		role="tablist"
		aria-orientation={orientation}
	>
		{#each tabs as tab, index (tab.id)}
			{@const isActive = tab.id === currentActiveTab}
			{@const isFocused = index === focusedTabIndex}
			<button
				class="gr-tabs__tab"
				class:gr-tabs__tab--active={isActive}
				class:gr-tabs__tab--disabled={tab.disabled}
				role="tab"
				data-tab-index={index}
				tabindex={tab.disabled ? -1 : isFocused ? 0 : -1}
				aria-selected={isActive}
				aria-disabled={tab.disabled}
				aria-controls={`${tabsId()}-panel-${tab.id}`}
				id={`${tabsId()}-tab-${tab.id}`}
				onclick={() => selectTab(tab.id)}
				onkeydown={(e) => handleTabKeydown(e, index)}
				onfocus={() => (focusedTabIndex = index)}
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<div class="gr-tabs__panels">
		{#each tabs as tab (tab.id)}
			{@const isActive = tab.id === currentActiveTab}
			<div
				class="gr-tabs__panel"
				class:gr-tabs__panel--active={isActive}
				role="tabpanel"
				id={`${tabsId()}-panel-${tab.id}`}
				aria-labelledby={`${tabsId()}-tab-${tab.id}`}
				tabindex={isActive ? 0 : -1}
				hidden={!isActive}
			>
				{#if tab.content && isActive}
					{@render tab.content()}
				{/if}
			</div>
		{/each}
	</div>
</div>
