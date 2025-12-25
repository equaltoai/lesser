/**
 * Headless Tabs Primitive
 *
 * Provides accessible tabs behavior without any styling.
 * Features:
 * - Arrow key navigation
 * - Home/End keys
 * - Automatic/Manual activation
 * - ARIA attributes
 * - Keyboard focus management
 * - Orientation support (horizontal/vertical)
 *
 * @module @equaltoai/greater-components-headless/tabs
 */

import { generateId } from '../utils/id';
import type { Action } from '../types/common';

/**
 * Tabs orientation
 */
export type TabsOrientation = 'horizontal' | 'vertical';

/**
 * Tab item data
 */
export interface TabItem {
	element: HTMLElement;
	index: number;
	disabled: boolean;
}

/**
 * Panel item data
 */
export interface PanelItem {
	element: HTMLElement;
	index: number;
}

/**
 * Tabs state
 */
export interface TabsState {
	/**
	 * Currently active tab index
	 */
	activeTab: number;

	/**
	 * Registered tab elements (for testing/inspection)
	 */
	tabs: HTMLElement[];

	/**
	 * Registered panel elements (for testing/inspection)
	 */
	panels: HTMLElement[];

	/**
	 * Tabs orientation
	 */
	orientation: TabsOrientation;

	/**
	 * Activate tab on focus (vs manual activation)
	 */
	activateOnFocus: boolean;

	/**
	 * Loop navigation
	 */
	loop: boolean;

	/**
	 * Tabs root ID
	 */
	id: string;

	/**
	 * Currently focused tab index
	 */
	focusedIndex: number;
}

/**
 * Tab action options
 */
export interface TabActionOptions {
	index: number;
	disabled?: boolean;
}

/**
 * Panel action options
 */
export interface PanelActionOptions {
	index: number;
}

/**
 * Tabs configuration
 */
export interface TabsConfig {
	/**
	 * Initial active tab index
	 */
	defaultTab?: number;

	/**
	 * Custom tabs ID
	 */
	id?: string;

	/**
	 * Orientation of tabs
	 */
	orientation?: TabsOrientation;

	/**
	 * Activate tab on focus (automatic activation)
	 */
	activateOnFocus?: boolean;

	/**
	 * Loop navigation (arrow keys wrap around)
	 */
	loop?: boolean;

	/**
	 * Called when active tab changes
	 */
	onChange?: (index: number) => void;

	/**
	 * Called when component is destroyed
	 */
	onDestroy?: () => void;
}

/**
 * Tabs actions
 */
export interface TabsActions {
	/**
	 * Action for tabs list container
	 */
	tabList: Action<HTMLElement>;

	/**
	 * Action for individual tab
	 */
	tab: (node: HTMLElement, options: TabActionOptions) => ReturnType<Action<HTMLElement>>;

	/**
	 * Action for tab panel
	 */
	panel: (node: HTMLElement, options: PanelActionOptions) => ReturnType<Action<HTMLElement>>;
}

/**
 * Tabs helpers
 */
export interface TabsHelpers {
	/**
	 * Set active tab by index
	 */
	setActiveTab: (index: number) => void;

	/**
	 * Focus next tab
	 */
	focusNext: () => void;

	/**
	 * Focus previous tab
	 */
	focusPrevious: () => void;

	/**
	 * Focus first tab
	 */
	focusFirst: () => void;

	/**
	 * Focus last tab
	 */
	focusLast: () => void;

	/**
	 * Set orientation
	 */
	setOrientation: (orientation: TabsOrientation) => void;
}

/**
 * Create a headless tabs primitive
 */
export function createTabs(config: TabsConfig = {}) {
	const {
		defaultTab = 0,
		id: customId,
		orientation = 'horizontal',
		activateOnFocus = false,
		loop = true,
		onChange,
		onDestroy,
	} = config;

	const tabs: TabItem[] = [];
	const panels: PanelItem[] = [];
	let listElement: HTMLElement | null = null;

	// Reactive state (works in both Svelte and test environments)
	const internalState: TabsState = {
		activeTab: defaultTab,
		tabs: [], // Will be dynamically updated by Proxy getter
		panels: [], // Will be dynamically updated by Proxy getter
		orientation,
		activateOnFocus,
		loop,
		id: customId || generateId('tabs'),
		focusedIndex: -1,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof TabsState, value) {
			const oldValue = target[prop];
			target[prop] = value as never;
			if (prop === 'activeTab' && oldValue !== value) {
				onChange?.(value as number);
				updateDOM();
			}
			if (prop === 'orientation' && listElement) {
				listElement.setAttribute('aria-orientation', value as string);
			}
			return true;
		},
		get(target, prop: keyof TabsState) {
			if (prop === 'tabs') {
				// Return just the elements for easier testing/usage
				return tabs.map((t) => t.element);
			}
			if (prop === 'panels') {
				// Return just the elements for easier testing/usage
				return panels.map((p) => p.element);
			}
			return target[prop];
		},
	});

	/**
	 * Update DOM for all tabs and panels
	 */
	function updateDOM() {
		for (const tab of tabs) {
			const isActive = tab.index === state.activeTab;
			tab.element.setAttribute('aria-selected', String(isActive));
			tab.element.tabIndex = isActive && !tab.disabled ? 0 : -1;
		}
		for (const panel of panels) {
			const isActive = panel.index === state.activeTab;
			panel.element.hidden = !isActive;
		}
	}

	/**
	 * Get non-disabled tabs
	 */
	function getEnabledTabs() {
		return tabs.filter((tab) => !tab.disabled);
	}

	/**
	 * Set active tab by index
	 */
	function setActiveTab(index: number) {
		if (state.activeTab === index) return;

		// Allow setting active tab even if no tabs are registered yet
		// but check if the tab exists and is not disabled
		if (tabs.length > 0) {
			const tab = tabs.find((t) => t.index === index && !t.disabled);
			if (!tab) return;
		}

		state.activeTab = index;
	}

	/**
	 * Focus tab at index
	 */
	function focusTab(index: number, activate = false) {
		const enabledTabs = getEnabledTabs();
		const tab = enabledTabs.find((t) => t.index === index);
		if (!tab) return;

		state.focusedIndex = index;
		tab.element.focus();

		// Manually trigger focus event for JSDOM compatibility
		tab.element.dispatchEvent(new FocusEvent('focus', { bubbles: false }));

		// Automatic activation or forced activation (e.g., Home/End keys)
		if (state.activateOnFocus || activate) {
			setActiveTab(index);
		}
	}

	/**
	 * Focus next tab
	 */
	function focusNext() {
		const enabledTabs = getEnabledTabs();
		if (enabledTabs.length === 0) return;

		const currentIndex = enabledTabs.findIndex((t) => t.index === state.focusedIndex);
		let nextIndex = currentIndex + 1;

		if (nextIndex >= enabledTabs.length) {
			nextIndex = state.loop ? 0 : enabledTabs.length - 1;
		}

		const nextTab = enabledTabs[nextIndex];
		if (nextTab) {
			focusTab(nextTab.index);
		}
	}

	/**
	 * Focus previous tab
	 */
	function focusPrevious() {
		const enabledTabs = getEnabledTabs();
		if (enabledTabs.length === 0) return;

		const currentIndex = enabledTabs.findIndex((t) => t.index === state.focusedIndex);
		let prevIndex = currentIndex - 1;

		if (prevIndex < 0) {
			prevIndex = state.loop ? enabledTabs.length - 1 : 0;
		}

		const prevTab = enabledTabs[prevIndex];
		if (prevTab) {
			focusTab(prevTab.index);
		}
	}

	/**
	 * Focus first tab
	 */
	function focusFirst(activate = false) {
		const enabledTabs = getEnabledTabs();
		const firstTab = enabledTabs[0];
		if (firstTab) {
			focusTab(firstTab.index, activate);
		}
	}

	/**
	 * Focus last tab
	 */
	function focusLast(activate = false) {
		const enabledTabs = getEnabledTabs();
		const lastTab = enabledTabs[enabledTabs.length - 1];
		if (lastTab) {
			focusTab(lastTab.index, activate);
		}
	}

	/**
	 * Set orientation
	 */
	function setOrientation(newOrientation: TabsOrientation) {
		state.orientation = newOrientation;
	}

	/**
	 * Handle keyboard navigation
	 */
	function handleKeyDown(event: KeyboardEvent) {
		const isVertical = state.orientation === 'vertical';
		const nextKey = isVertical ? 'ArrowDown' : 'ArrowRight';
		const prevKey = isVertical ? 'ArrowUp' : 'ArrowLeft';

		switch (event.key) {
			case nextKey:
				event.preventDefault();
				focusNext();
				break;

			case prevKey:
				event.preventDefault();
				focusPrevious();
				break;

			case 'Home':
				event.preventDefault();
				focusFirst(true); // Always activate with Home key
				break;

			case 'End':
				event.preventDefault();
				focusLast(true); // Always activate with End key
				break;

			case 'Enter':
			case ' ':
				if (!state.activateOnFocus) {
					event.preventDefault();
					if (state.focusedIndex >= 0) {
						setActiveTab(state.focusedIndex);
					}
				}
				break;
		}
	}

	/**
	 * Tab list action
	 */
	const tabList: Action<HTMLElement> = (node: HTMLElement) => {
		listElement = node;

		node.addEventListener('keydown', handleKeyDown);

		node.setAttribute('role', 'tablist');
		node.setAttribute('aria-orientation', state.orientation);

		return {
			destroy() {
				node.removeEventListener('keydown', handleKeyDown);
				listElement = null;
				tabs.length = 0;
				panels.length = 0;
				onDestroy?.();
			},
		};
	};

	/**
	 * Tab action
	 */
	function tab(node: HTMLElement, options: TabActionOptions): ReturnType<Action<HTMLElement>> {
		const { index, disabled = false } = options;
		const tabData: TabItem = { element: node, index, disabled };
		tabs.push(tabData);

		const isActive = () => state.activeTab === index;

		function handleClick() {
			if (disabled) return;
			setActiveTab(index);
			node.focus();
		}

		function handleFocus() {
			state.focusedIndex = index;
			if (state.activateOnFocus && !disabled) {
				setActiveTab(index);
			}
		}

		node.addEventListener('click', handleClick);
		node.addEventListener('focus', handleFocus);
		node.addEventListener('keydown', handleKeyDown); // Add keyboard handler to each tab

		node.setAttribute('role', 'tab');
		node.setAttribute('aria-selected', String(isActive()));
		node.setAttribute('aria-controls', `panel-${index}`);
		node.setAttribute('id', `tab-${index}`);
		node.tabIndex = isActive() ? 0 : -1;

		if (disabled) {
			node.setAttribute('aria-disabled', 'true');
			node.tabIndex = -1;
		}

		return {
			update() {
				node.setAttribute('aria-selected', String(isActive()));
				node.tabIndex = isActive() && !disabled ? 0 : -1;
			},
			destroy() {
				node.removeEventListener('click', handleClick);
				node.removeEventListener('focus', handleFocus);
				node.removeEventListener('keydown', handleKeyDown);

				const tabIndex = tabs.findIndex((t) => t.element === node);
				if (tabIndex !== -1) {
					tabs.splice(tabIndex, 1);
				}

				// Call onDestroy when all tabs and panels are cleaned up
				if (tabs.length === 0 && panels.length === 0) {
					onDestroy?.();
				}
			},
		};
	}

	/**
	 * Panel action
	 */
	function panel(node: HTMLElement, options: PanelActionOptions): ReturnType<Action<HTMLElement>> {
		const { index } = options;
		const panelData: PanelItem = { element: node, index };
		panels.push(panelData);

		const isActive = () => state.activeTab === index;

		node.setAttribute('role', 'tabpanel');
		node.setAttribute('aria-labelledby', `tab-${index}`);
		node.setAttribute('id', `panel-${index}`);
		node.tabIndex = 0;

		// Hide inactive panels
		node.hidden = !isActive();

		return {
			update() {
				node.hidden = !isActive();
			},
			destroy() {
				const panelIndex = panels.findIndex((p) => p.element === node);
				if (panelIndex !== -1) {
					panels.splice(panelIndex, 1);
				}

				// Call onDestroy when all tabs and panels are cleaned up
				if (tabs.length === 0 && panels.length === 0) {
					onDestroy?.();
				}
			},
		};
	}

	const actions: TabsActions = {
		tabList,
		tab,
		panel,
	};

	const helpers: TabsHelpers = {
		setActiveTab,
		focusNext,
		focusPrevious,
		focusFirst,
		focusLast,
		setOrientation,
	};

	return {
		state,
		actions,
		helpers,
	};
}
