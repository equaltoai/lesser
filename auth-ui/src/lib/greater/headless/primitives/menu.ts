/**
 * Headless Menu Primitive
 *
 * Provides accessible dropdown menu behavior without any styling.
 * Features:
 * - Keyboard navigation (Arrow keys, Home, End)
 * - Typeahead search
 * - Nested submenus
 * - ARIA attributes
 * - Click outside to close
 * - ESC to close
 * - Focus management
 *
 * @module @equaltoai/greater-components-headless/menu
 */

import { generateId } from '../utils/id';
import type { Action } from '../types/common';

/**
 * Menu state
 */
export interface MenuState {
	/**
	 * Whether menu is open
	 */
	open: boolean;

	/**
	 * Menu ID
	 */
	id: string;

	/**
	 * Currently focused/active item index
	 */
	focusedIndex: number;

	/**
	 * Active item index (alias for focusedIndex)
	 */
	activeIndex: number;

	/**
	 * Registered menu items
	 */
	items: HTMLElement[];

	/**
	 * Whether menu should close on select
	 */
	closeOnSelect: boolean;

	/**
	 * Whether menu should loop navigation
	 */
	loop: boolean;

	/**
	 * Typeahead search buffer
	 */
	typeaheadBuffer: string;
}

/**
 * Menu configuration
 */
export interface MenuConfig {
	/**
	 * Initial open state
	 */
	open?: boolean;

	/**
	 * Custom menu ID
	 */
	id?: string;

	/**
	 * Close on item select
	 */
	closeOnSelect?: boolean;

	/**
	 * Loop navigation (wrap around)
	 */
	loop?: boolean;

	/**
	 * Enable typeahead search
	 */
	typeAhead?: boolean;

	/**
	 * Typeahead timeout (ms)
	 */
	typeaheadTimeout?: number;

	/**
	 * Menu placement - extended options for dropdown positioning
	 */
	placement?:
		| 'bottom'
		| 'bottom-start'
		| 'bottom-end'
		| 'top'
		| 'top-start'
		| 'top-end'
		| 'left'
		| 'right';

	/**
	 * Offset from trigger element in pixels
	 */
	offset?: number;

	/**
	 * Enable smart positioning (auto-flip on viewport overflow)
	 */
	smartPositioning?: boolean;

	/**
	 * Called when open state changes
	 */
	onOpenChange?: (open: boolean) => void;

	/**
	 * Called when menu opens
	 */
	onOpen?: () => void;

	/**
	 * Called when menu closes
	 */
	onClose?: () => void;

	/**
	 * Called when item is selected
	 */
	onSelect?: (value: string) => void;

	/**
	 * Called on destroy
	 */
	onDestroy?: () => void;
}

/**
 * Menu actions
 */
export interface MenuActions {
	/**
	 * Action for menu trigger button
	 */
	trigger: Action<HTMLButtonElement>;

	/**
	 * Action for menu content
	 */
	menu: Action<HTMLElement>;

	/**
	 * Action for menu content (alias for menu)
	 */
	content: Action<HTMLElement>;

	/**
	 * Action for menu items
	 */
	item: (
		node: HTMLElement,
		options?: string | { value?: string; disabled?: boolean; onClick?: () => void }
	) => { destroy: () => void };

	/**
	 * Action for menu separator
	 */
	separator: Action<HTMLElement>;
}

/**
 * Menu helpers
 */
export interface MenuHelpers {
	/**
	 * Open the menu
	 */
	open: () => void;

	/**
	 * Close the menu
	 */
	close: () => void;

	/**
	 * Toggle the menu
	 */
	toggle: () => void;

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
	 * Handle keyboard events
	 */
	handleKeyDown: (event: KeyboardEvent) => void;

	/**
	 * Set active index
	 */
	setActiveIndex: (index: number) => void;
}

/**
 * Create a headless menu primitive
 */
export function createMenu(config: MenuConfig = {}) {
	const {
		open: initialOpen = false,
		id: customId,
		closeOnSelect: closeOnSelectProp = true,
		loop: loopProp = true,
		typeAhead: enableTypeahead = true,
		typeaheadTimeout = 500,
		placement = 'bottom-start',
		offset: _offsetProp = 4,
		smartPositioning: _smartPositioning = true,
		onOpen,
		onClose,
		onOpenChange,
		onSelect: _onSelect,
		onDestroy,
	} = config;

	let menuItems: Array<{
		element: HTMLElement;
		value: string;
		disabled: boolean;
		onClick?: () => void;
	}> = [];

	// Reactive state using Proxy
	const internalState: MenuState = {
		open: initialOpen,
		id: customId || generateId('menu'),
		focusedIndex: -1,
		activeIndex: -1, // Alias for focusedIndex
		items: [],
		closeOnSelect: closeOnSelectProp,
		loop: loopProp,
		typeaheadBuffer: '',
	};

	const state = new Proxy(internalState, {
		set(target, prop: keyof MenuState, value) {
			target[prop] = value as never;
			// Keep activeIndex and focusedIndex in sync
			if (prop === 'focusedIndex') {
				target.activeIndex = value as number;
			} else if (prop === 'activeIndex') {
				target.focusedIndex = value as number;
			}
			// Call onOpenChange when open changes
			if (prop === 'open') {
				onOpenChange?.(value as boolean);
			}
			updateDOM();
			return true;
		},
		get(target, prop: keyof MenuState) {
			// Ensure items reflects current menuItems
			if (prop === 'items') {
				return menuItems.map((item) => item.element);
			}
			return target[prop];
		},
	});

	let triggerElement: HTMLButtonElement | null = null;
	let menuElement: HTMLElement | null = null;
	let typeaheadTimeoutId: number | undefined = undefined;

	/**
	 * Update DOM when state changes
	 */
	function updateDOM() {
		// Update trigger aria-expanded
		if (triggerElement) {
			triggerElement.setAttribute('aria-expanded', String(state.open));
		}

		// Update items data-active attributes
		menuItems.forEach((item, idx) => {
			if (idx === state.focusedIndex) {
				item.element.setAttribute('data-active', 'true');
			} else {
				item.element.setAttribute('data-active', 'false');
			}
		});
	}

	/**
	 * Open the menu
	 */
	function open() {
		if (state.open) return;

		state.open = true;
		state.focusedIndex = -1;

		// Focus first item - use setTimeout only if items don't exist yet
		if (menuItems.length > 0) {
			focusFirst();
		} else {
			setTimeout(() => {
				focusFirst();
			}, 0);
		}

		onOpen?.();
	}

	/**
	 * Close the menu
	 */
	function close() {
		if (!state.open) return;

		state.open = false;
		state.focusedIndex = -1;
		state.typeaheadBuffer = '';

		// Return focus to trigger
		if (triggerElement) {
			triggerElement.focus();
		}

		onClose?.();
	}

	/**
	 * Toggle the menu
	 */
	function toggle() {
		if (state.open) {
			close();
		} else {
			open();
		}
	}

	/**
	 * Get non-disabled menu items
	 */
	function getEnabledItems() {
		return menuItems.filter((item) => !item.disabled);
	}

	/**
	 * Focus item at index (index in menuItems array, not enabled items)
	 */
	function focusItem(index: number) {
		if (index < 0 || index >= menuItems.length) return;

		const item = menuItems[index];
		if (!item || item.disabled) return;

		state.focusedIndex = index;
		item.element.focus();
		// Manually trigger focus event for JSDOM compatibility
		item.element.dispatchEvent(new FocusEvent('focus', { bubbles: false }));
	}

	/**
	 * Focus next item
	 */
	function focusNext() {
		const enabledItems = getEnabledItems();
		if (enabledItems.length === 0) return;

		let nextIndex = state.focusedIndex + 1;
		if (nextIndex >= menuItems.length) {
			nextIndex = state.loop ? 0 : menuItems.length - 1;
		}

		// Skip disabled items
		while (nextIndex !== state.focusedIndex && menuItems[nextIndex]?.disabled) {
			nextIndex++;
			if (nextIndex >= menuItems.length) {
				nextIndex = state.loop ? 0 : menuItems.length - 1;
				break;
			}
		}

		focusItem(nextIndex);
	}

	/**
	 * Focus previous item
	 */
	function focusPrevious() {
		const enabledItems = getEnabledItems();
		if (enabledItems.length === 0) return;

		let prevIndex = state.focusedIndex - 1;
		if (prevIndex < 0) {
			prevIndex = state.loop ? menuItems.length - 1 : 0;
		}

		// Skip disabled items
		while (prevIndex !== state.focusedIndex && menuItems[prevIndex]?.disabled) {
			prevIndex--;
			if (prevIndex < 0) {
				prevIndex = state.loop ? menuItems.length - 1 : 0;
				break;
			}
		}

		focusItem(prevIndex);
	}

	/**
	 * Focus first item
	 */
	function focusFirst() {
		// Find first enabled item
		for (let i = 0; i < menuItems.length; i++) {
			const item = menuItems[i];
			if (item && !item.disabled) {
				focusItem(i);
				return;
			}
		}
	}

	/**
	 * Focus last item
	 */
	function focusLast() {
		// Find last enabled item
		for (let i = menuItems.length - 1; i >= 0; i--) {
			const item = menuItems[i];
			if (item && !item.disabled) {
				focusItem(i);
				return;
			}
		}
	}

	/**
	 * Handle typeahead search
	 */
	function handleTypeahead(char: string) {
		if (!enableTypeahead) return;

		// Clear existing timeout
		if (typeaheadTimeoutId !== undefined) {
			window.clearTimeout(typeaheadTimeoutId);
		}

		// Add character to buffer
		state.typeaheadBuffer += char.toLowerCase();

		// Find matching item in menuItems (get actual index, not enabled items index)
		const matchIndex = menuItems.findIndex((item) => {
			if (item.disabled) return false;
			const text = item.element.textContent?.toLowerCase() || '';
			return text.startsWith(state.typeaheadBuffer);
		});

		if (matchIndex !== -1) {
			focusItem(matchIndex);
		}

		// Clear buffer after timeout
		typeaheadTimeoutId = window.setTimeout(() => {
			state.typeaheadBuffer = '';
			typeaheadTimeoutId = undefined;
		}, typeaheadTimeout);
	}

	/**
	 * Handle menu keyboard events
	 */
	function handleMenuKeyDown(event: KeyboardEvent) {
		if (!state.open) return;

		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault();
				focusNext();
				break;

			case 'ArrowUp':
				event.preventDefault();
				focusPrevious();
				break;

			case 'Home':
				event.preventDefault();
				focusFirst();
				break;

			case 'End':
				event.preventDefault();
				focusLast();
				break;

			case 'Escape':
				event.preventDefault();
				close();
				break;

			case 'Tab':
				event.preventDefault();
				close();
				break;

			default:
				// Typeahead
				if (event.key.length === 1 && !event.ctrlKey && !event.metaKey) {
					handleTypeahead(event.key);
				}
				break;
		}
	}

	/**
	 * Handle click outside
	 */
	function handleClickOutside(event: MouseEvent) {
		if (!state.open) return;

		const target = event.target as Node;

		// Close if click is outside menu element
		if (menuElement && !menuElement.contains(target)) {
			// Also check trigger if it exists
			if (!triggerElement || !triggerElement.contains(target)) {
				close();
			}
		}
	}

	/**
	 * Trigger action
	 */
	const trigger: Action<HTMLButtonElement> = (node: HTMLButtonElement) => {
		triggerElement = node;

		function handleClick() {
			toggle();
		}

		function handleKeyDown(event: KeyboardEvent) {
			if (event.key === 'ArrowDown') {
				event.preventDefault();
				open();
			} else if (event.key === 'ArrowUp') {
				event.preventDefault();
				open();
				// Focus last item when opening with arrow up
				setTimeout(focusLast, 0);
			} else if (event.key === 'Escape') {
				event.preventDefault();
				close();
			}
		}

		node.addEventListener('click', handleClick);
		node.addEventListener('keydown', handleKeyDown);

		node.setAttribute('aria-haspopup', 'true');
		node.setAttribute('aria-expanded', String(state.open));
		node.setAttribute('aria-controls', state.id);

		return {
			update() {
				node.setAttribute('aria-expanded', String(state.open));
			},
			destroy() {
				node.removeEventListener('click', handleClick);
				node.removeEventListener('keydown', handleKeyDown);
				triggerElement = null;
				onDestroy?.();
			},
		};
	};

	/**
	 * Menu action
	 */
	const menu: Action<HTMLElement> = (node: HTMLElement) => {
		menuElement = node;

		node.addEventListener('keydown', handleMenuKeyDown);
		document.addEventListener('click', handleClickOutside);

		node.setAttribute('role', 'menu');
		node.setAttribute('id', state.id);
		node.setAttribute('data-placement', placement);
		node.tabIndex = -1;

		return {
			destroy() {
				node.removeEventListener('keydown', handleMenuKeyDown);
				document.removeEventListener('click', handleClickOutside);
				menuElement = null;
				menuItems = [];
				onDestroy?.();
			},
		};
	};

	/**
	 * Menu item action
	 */
	function item(
		node: HTMLElement,
		options: string | { value?: string; disabled?: boolean; onClick?: () => void } = {}
	) {
		const normalizedOptions = typeof options === 'string' ? { value: options } : options;
		const { value = '', disabled = false, onClick: onClickHandler } = normalizedOptions;

		// Check if item already exists and update it
		const existingIndex = menuItems.findIndex((item) => item.element === node);
		if (existingIndex !== -1) {
			const existingItem = menuItems[existingIndex];
			if (existingItem) {
				existingItem.value = value;
				existingItem.disabled = disabled;
				existingItem.onClick = onClickHandler;
			}
			if (disabled) {
				node.setAttribute('aria-disabled', 'true');
			} else {
				node.removeAttribute('aria-disabled');
			}
			// Return a destroy function that removes this item
			return {
				destroy() {
					const idx = menuItems.findIndex((item) => item.element === node);
					if (idx !== -1) {
						menuItems.splice(idx, 1);
					}
				},
			};
		}

		const itemData = { element: node, value, disabled, onClick: onClickHandler };
		menuItems.push(itemData);

		function handleClick() {
			if (itemData.disabled) return;

			itemData.onClick?.();
			_onSelect?.(itemData.value);

			if (state.closeOnSelect) {
				close();
			}
		}

		function handleKeyDown(event: KeyboardEvent) {
			if (event.key === 'Enter' || event.key === ' ') {
				event.preventDefault();
				if (!itemData.disabled) {
					handleClick();
				}
			}
		}

		node.addEventListener('click', handleClick);
		node.addEventListener('keydown', handleKeyDown);

		node.setAttribute('role', 'menuitem');
		node.tabIndex = -1;

		if (disabled) {
			node.setAttribute('aria-disabled', 'true');
		}

		return {
			destroy() {
				node.removeEventListener('click', handleClick);
				node.removeEventListener('keydown', handleKeyDown);

				const index = menuItems.findIndex((item) => item.element === node);
				if (index !== -1) {
					menuItems.splice(index, 1);
				}
			},
		};
	}

	/**
	 * Separator action
	 */
	const separator: Action<HTMLElement> = (node: HTMLElement) => {
		node.setAttribute('role', 'separator');
		node.setAttribute('aria-orientation', 'horizontal');

		return {
			destroy() {},
		};
	};

	/**
	 * Handle keyboard events (exposed helper)
	 */
	function handleKeyDown(event: KeyboardEvent | { key: string }) {
		if (!state.open) return;

		// Check if preventDefault exists (for test compatibility)
		const preventDefault =
			'preventDefault' in event && typeof event.preventDefault === 'function'
				? () => event.preventDefault()
				: () => {};

		switch (event.key) {
			case 'ArrowDown':
				preventDefault();
				focusNext();
				break;
			case 'ArrowUp':
				preventDefault();
				focusPrevious();
				break;
			case 'Home':
				preventDefault();
				focusFirst();
				break;
			case 'End':
				preventDefault();
				focusLast();
				break;
			case 'Escape':
				preventDefault();
				close();
				break;
			default:
				// Handle type-ahead for printable characters
				if (event.key.length === 1) {
					handleTypeahead(event.key);
				}
				break;
		}
	}

	/**
	 * Set active index (exposed helper)
	 */
	function setActiveIndex(index: number) {
		if (index >= 0 && index < menuItems.length) {
			state.focusedIndex = index;
			const item = menuItems[index];
			if (item && !item.disabled) {
				item.element.focus();
			}
		}
	}

	const actions: MenuActions = {
		trigger,
		menu,
		content: menu, // Alias for menu
		item,
		separator,
	};

	const helpers: MenuHelpers = {
		open,
		close,
		toggle,
		focusNext,
		focusPrevious,
		focusFirst,
		focusLast,
		handleKeyDown,
		setActiveIndex,
	};

	return {
		state,
		actions,
		helpers,
	};
}
