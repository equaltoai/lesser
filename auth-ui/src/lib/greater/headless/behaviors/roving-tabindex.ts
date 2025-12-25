/**
 * Roving Tabindex Behavior
 *
 * Framework-agnostic utility for arrow key navigation in composite widgets.
 * Implements the roving tabindex pattern for menus, toolbars, tabs, and listboxes.
 *
 * @module @equaltoai/greater-components-headless/behaviors/roving-tabindex
 */

/**
 * Orientation for navigation
 */
export type RovingOrientation = 'horizontal' | 'vertical' | 'both';

/**
 * Roving tabindex configuration
 */
export interface RovingTabindexConfig {
	/**
	 * Navigation orientation
	 * - 'horizontal': Left/Right arrows
	 * - 'vertical': Up/Down arrows
	 * - 'both': All arrow keys
	 * @default 'vertical'
	 */
	orientation?: RovingOrientation;

	/**
	 * Whether navigation loops at boundaries
	 * @default true
	 */
	loop?: boolean;

	/**
	 * Initial focused index
	 * @default 0
	 */
	initialIndex?: number;

	/**
	 * Whether Home/End keys are enabled
	 * @default true
	 */
	homeEndKeys?: boolean;

	/**
	 * Callback when focused index changes
	 */
	onFocusChange?: (index: number, element: HTMLElement) => void;

	/**
	 * Callback when item is activated (Enter/Space)
	 */
	onActivate?: (index: number, element: HTMLElement) => void;

	/**
	 * Function to determine if an item is disabled
	 */
	isDisabled?: (element: HTMLElement, index: number) => boolean;
}

/**
 * Roving tabindex state
 */
export interface RovingTabindexState {
	/**
	 * Currently focused index
	 */
	focusedIndex: number;

	/**
	 * Registered items
	 */
	items: HTMLElement[];

	/**
	 * Whether the roving tabindex is active
	 */
	active: boolean;
}

/**
 * Roving tabindex return type
 */
export interface RovingTabindex {
	/**
	 * Current state
	 */
	state: RovingTabindexState;

	/**
	 * Register an item
	 */
	registerItem: (element: HTMLElement, index?: number) => void;

	/**
	 * Unregister an item
	 */
	unregisterItem: (element: HTMLElement) => void;

	/**
	 * Set focused index
	 */
	setFocusedIndex: (index: number, focus?: boolean) => void;

	/**
	 * Focus next item
	 */
	focusNext: () => void;

	/**
	 * Focus previous item
	 */
	focusPrevious: () => void;

	/**
	 * Focus first item
	 */
	focusFirst: () => void;

	/**
	 * Focus last item
	 */
	focusLast: () => void;

	/**
	 * Handle keyboard event (attach to container)
	 */
	handleKeyDown: (event: KeyboardEvent) => void;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<RovingTabindexConfig>) => void;

	/**
	 * Destroy and clean up
	 */
	destroy: () => void;
}

/**
 * Default disabled check
 */
function defaultIsDisabled(element: HTMLElement): boolean {
	return (
		element.hasAttribute('disabled') ||
		element.getAttribute('aria-disabled') === 'true' ||
		element.hasAttribute('data-disabled')
	);
}

/**
 * Create a roving tabindex manager
 */
export function createRovingTabindex(config: RovingTabindexConfig = {}): RovingTabindex {
	let currentConfig: Required<RovingTabindexConfig> = {
		orientation: config.orientation ?? 'vertical',
		loop: config.loop ?? true,
		initialIndex: config.initialIndex ?? 0,
		homeEndKeys: config.homeEndKeys ?? true,
		onFocusChange: config.onFocusChange ?? (() => {}),
		onActivate: config.onActivate ?? (() => {}),
		isDisabled: config.isDisabled ?? defaultIsDisabled,
	};

	const state: RovingTabindexState = {
		focusedIndex: currentConfig.initialIndex,
		items: [],
		active: true,
	};

	/**
	 * Update tabindex values for all items
	 */
	function updateTabindices(): void {
		state.items.forEach((item, index) => {
			item.tabIndex = index === state.focusedIndex ? 0 : -1;
		});
	}

	/**
	 * Get enabled items
	 */
	function getEnabledItems(): { element: HTMLElement; index: number }[] {
		return state.items
			.map((element, index) => ({ element, index }))
			.filter(({ element, index }) => !currentConfig.isDisabled(element, index));
	}

	/**
	 * Find next enabled index
	 */
	function findNextEnabledIndex(startIndex: number, direction: 1 | -1): number {
		const enabledItems = getEnabledItems();
		if (enabledItems.length === 0) return -1;

		let currentIndex = startIndex;
		const totalItems = state.items.length;

		for (let i = 0; i < totalItems; i++) {
			currentIndex += direction;

			if (currentConfig.loop) {
				if (currentIndex >= totalItems) currentIndex = 0;
				if (currentIndex < 0) currentIndex = totalItems - 1;
			} else {
				if (currentIndex >= totalItems || currentIndex < 0) {
					return state.focusedIndex;
				}
			}

			const item = state.items[currentIndex];
			if (item && !currentConfig.isDisabled(item, currentIndex)) {
				return currentIndex;
			}
		}

		return state.focusedIndex;
	}

	/**
	 * Register an item
	 */
	function registerItem(element: HTMLElement, index?: number): void {
		if (index !== undefined && index >= 0) {
			state.items[index] = element;
		} else {
			state.items.push(element);
		}
		updateTabindices();
	}

	/**
	 * Unregister an item
	 */
	function unregisterItem(element: HTMLElement): void {
		const index = state.items.indexOf(element);
		if (index !== -1) {
			state.items.splice(index, 1);

			// Adjust focused index if needed
			if (state.focusedIndex >= state.items.length) {
				state.focusedIndex = Math.max(0, state.items.length - 1);
			}

			updateTabindices();
		}
	}

	/**
	 * Set focused index
	 */
	function setFocusedIndex(index: number, focus = true): void {
		if (index < 0 || index >= state.items.length) return;

		const item = state.items[index];
		if (!item || currentConfig.isDisabled(item, index)) return;

		state.focusedIndex = index;
		updateTabindices();

		if (focus) {
			item.focus();
		}

		currentConfig.onFocusChange(index, item);
	}

	/**
	 * Focus next item
	 */
	function focusNext(): void {
		const nextIndex = findNextEnabledIndex(state.focusedIndex, 1);
		if (nextIndex !== -1) {
			setFocusedIndex(nextIndex);
		}
	}

	/**
	 * Focus previous item
	 */
	function focusPrevious(): void {
		const prevIndex = findNextEnabledIndex(state.focusedIndex, -1);
		if (prevIndex !== -1) {
			setFocusedIndex(prevIndex);
		}
	}

	/**
	 * Focus first enabled item
	 */
	function focusFirst(): void {
		const enabledItems = getEnabledItems();
		if (enabledItems.length > 0) {
			const item = enabledItems[0];
			if (item) {
				setFocusedIndex(item.index);
			}
		}
	}

	/**
	 * Focus last enabled item
	 */
	function focusLast(): void {
		const enabledItems = getEnabledItems();
		if (enabledItems.length > 0) {
			const item = enabledItems[enabledItems.length - 1];
			if (item) {
				setFocusedIndex(item.index);
			}
		}
	}

	/**
	 * Handle keyboard navigation
	 */
	function handleKeyDown(event: KeyboardEvent): void {
		if (!state.active) return;

		const { orientation, homeEndKeys } = currentConfig;

		let handled = false;

		switch (event.key) {
			case 'ArrowDown':
				if (orientation === 'vertical' || orientation === 'both') {
					focusNext();
					handled = true;
				}
				break;

			case 'ArrowUp':
				if (orientation === 'vertical' || orientation === 'both') {
					focusPrevious();
					handled = true;
				}
				break;

			case 'ArrowRight':
				if (orientation === 'horizontal' || orientation === 'both') {
					focusNext();
					handled = true;
				}
				break;

			case 'ArrowLeft':
				if (orientation === 'horizontal' || orientation === 'both') {
					focusPrevious();
					handled = true;
				}
				break;

			case 'Home':
				if (homeEndKeys) {
					focusFirst();
					handled = true;
				}
				break;

			case 'End':
				if (homeEndKeys) {
					focusLast();
					handled = true;
				}
				break;

			case 'Enter':
			case ' ': {
				const currentItem = state.items[state.focusedIndex];
				if (currentItem) {
					currentConfig.onActivate(state.focusedIndex, currentItem);
					handled = true;
				}
				break;
			}
		}

		if (handled) {
			event.preventDefault();
			event.stopPropagation();
		}
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<RovingTabindexConfig>): void {
		currentConfig = {
			...currentConfig,
			...newConfig,
			onFocusChange: newConfig.onFocusChange ?? currentConfig.onFocusChange,
			onActivate: newConfig.onActivate ?? currentConfig.onActivate,
			isDisabled: newConfig.isDisabled ?? currentConfig.isDisabled,
		};
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		state.active = false;
		state.items = [];
		state.focusedIndex = 0;
	}

	return {
		state,
		registerItem,
		unregisterItem,
		setFocusedIndex,
		focusNext,
		focusPrevious,
		focusFirst,
		focusLast,
		handleKeyDown,
		updateConfig,
		destroy,
	};
}
