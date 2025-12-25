/**
 * Headless Button Primitive
 *
 * Provides all button behavior without any styling opinions.
 * Handles keyboard navigation, ARIA attributes, loading states, and accessibility.
 *
 * @example
 * ```svelte
 * <script>
 *   import { createButton } from 'src/lib/greater/headless/button';
 *
 *   const button = createButton({
 *     onClick: () => console.log('clicked'),
 *     disabled: false
 *   });
 * </script>
 *
 * <button use:button.actions.button class="my-custom-styles">
 *   Click me
 * </button>
 * ```
 *
 * @module @equaltoai/greater-components-headless/button
 */

import type { ActionReturn, BaseBuilderConfig } from '../types/common.js';
import { generateId } from '../utils/id.js';
import { isActivationKey } from '../utils/keyboard.js';

/**
 * Button configuration options
 */
export interface ButtonConfig extends BaseBuilderConfig {
	/**
	 * Button type attribute
	 * @defaultValue 'button'
	 */
	type?: 'button' | 'submit' | 'reset';

	/**
	 * Loading state - disables interaction and shows aria-busy
	 * @defaultValue false
	 */
	loading?: boolean;

	/**
	 * Whether the button is pressed (for toggle buttons)
	 * @defaultValue false
	 */
	pressed?: boolean;

	/**
	 * ARIA label for the button
	 */
	label?: string;

	/**
	 * Click handler
	 */
	onClick?: (event: MouseEvent) => void;

	/**
	 * Called when pressed state changes (for toggle buttons)
	 */
	onPressedChange?: (pressed: boolean) => void;

	/**
	 * Keyboard event handler
	 */
	onKeyDown?: (event: KeyboardEvent) => void;

	/**
	 * Focus event handler
	 */
	onFocus?: (event: FocusEvent) => void;

	/**
	 * Blur event handler
	 */
	onBlur?: (event: FocusEvent) => void;
}

/**
 * Button state
 */
export interface ButtonState {
	/**
	 * Whether the button is disabled
	 */
	disabled: boolean;

	/**
	 * Whether the button is in loading state
	 */
	loading: boolean;

	/**
	 * Whether the button is pressed (for toggle buttons)
	 */
	pressed: boolean;

	/**
	 * The button's ID
	 */
	id: string;

	/**
	 * Whether the button is currently focused
	 */
	focused: boolean;
}

/**
 * Button builder return type
 */
export interface ButtonBuilder {
	/**
	 * Current button state (reactive with Svelte 5 runes)
	 */
	state: ButtonState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: {
		/**
		 * Main button action - apply to button element
		 */
		button: (node: HTMLButtonElement) => ActionReturn;
	};

	/**
	 * Helper methods
	 */
	helpers: {
		/**
		 * Programmatically trigger the button click
		 */
		click: () => void;

		/**
		 * Programmatically focus the button
		 */
		focus: () => void;

		/**
		 * Programmatically blur the button
		 */
		blur: () => void;

		/**
		 * Toggle pressed state (for toggle buttons)
		 */
		toggle: () => void;

		/**
		 * Set disabled state
		 */
		setDisabled: (disabled: boolean) => void;

		/**
		 * Set loading state
		 */
		setLoading: (loading: boolean) => void;

		/**
		 * Set pressed state (for toggle buttons)
		 */
		setPressed: (pressed: boolean) => void;

		/**
		 * Clean up resources
		 */
		destroy: () => void;
	};
}

/**
 * Create a headless button
 *
 * @param config - Button configuration
 * @returns Button builder with state, actions, and helpers
 *
 * @public
 */
export function createButton(config: ButtonConfig = {}): ButtonBuilder {
	const {
		type = 'button',
		disabled: initialDisabled = false,
		loading: initialLoading = false,
		pressed: initialPressed = false,
		id: customId,
		label,
		onClick,
		onPressedChange,
		onKeyDown,
		onFocus,
		onBlur,
		onDestroy,
	} = config;

	let buttonElement: HTMLButtonElement | null = null;

	// Reactive state (works in both Svelte and test environments)
	const internalState: ButtonState = {
		disabled: initialDisabled,
		loading: initialLoading,
		pressed: initialPressed,
		id: customId || generateId('button'),
		focused: false,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof ButtonState, value) {
			const oldValue = target[prop];
			target[prop] = value as never;
			if (prop === 'pressed' && oldValue !== value) {
				onPressedChange?.(value as boolean);
			}
			updateDOM();
			return true;
		},
	});

	/**
	 * Update DOM when state changes
	 */
	function updateDOM() {
		if (buttonElement) {
			updateAriaAttributes(buttonElement);
		}
	}

	/**
	 * Handle button click
	 */
	function handleClick(event: MouseEvent) {
		if (state.disabled || state.loading) {
			event.preventDefault();
			event.stopPropagation();
			return;
		}

		onClick?.(event);
	}

	/**
	 * Handle keyboard events
	 */
	function handleKeyDown(event: KeyboardEvent) {
		if (state.disabled || state.loading) {
			return;
		}

		// Handle Space and Enter as activation keys
		if (isActivationKey(event)) {
			event.preventDefault();

			// Trigger click event
			if (buttonElement && onClick) {
				const clickEvent = new MouseEvent('click', {
					bubbles: true,
					cancelable: true,
				});
				handleClick(clickEvent);
			}
		}

		onKeyDown?.(event);
	}

	/**
	 * Handle focus
	 */
	function handleFocus(event: FocusEvent) {
		state.focused = true;
		onFocus?.(event);
	}

	/**
	 * Handle blur
	 */
	function handleBlur(event: FocusEvent) {
		state.focused = false;
		onBlur?.(event);
	}

	/**
	 * Main button action
	 */
	function button(node: HTMLButtonElement): ActionReturn {
		buttonElement = node;

		// Set initial attributes
		node.setAttribute('type', type);
		node.id = state.id;

		// Set ARIA attributes
		updateAriaAttributes(node);

		// Add event listeners
		node.addEventListener('click', handleClick);
		node.addEventListener('keydown', handleKeyDown);
		node.addEventListener('focus', handleFocus);
		node.addEventListener('blur', handleBlur);

		return {
			update() {
				// Called automatically by Svelte when state changes
				updateAriaAttributes(node);
			},
			destroy() {
				node.removeEventListener('click', handleClick);
				node.removeEventListener('keydown', handleKeyDown);
				node.removeEventListener('focus', handleFocus);
				node.removeEventListener('blur', handleBlur);
				buttonElement = null;
				onDestroy?.();
			},
		};
	}

	/**
	 * Update ARIA attributes based on state
	 */
	function updateAriaAttributes(node: HTMLButtonElement) {
		const isInteractive = !state.disabled && !state.loading;

		node.disabled = state.disabled || state.loading;
		node.setAttribute('aria-disabled', String(state.disabled || state.loading));
		node.setAttribute('aria-busy', String(state.loading));
		node.setAttribute('aria-pressed', String(state.pressed));

		if (label) {
			node.setAttribute('aria-label', label);
		}

		// Manage tabindex
		if (!isInteractive) {
			node.setAttribute('tabindex', '-1');
		} else {
			node.removeAttribute('tabindex');
		}
	}

	/**
	 * Programmatically trigger click
	 */
	function click() {
		if (buttonElement && !state.disabled && !state.loading) {
			buttonElement.click();
		}
	}

	/**
	 * Programmatically focus button
	 */
	function focus() {
		if (buttonElement && !state.disabled) {
			buttonElement.focus();
		}
	}

	/**
	 * Programmatically blur button
	 */
	function blur() {
		if (buttonElement) {
			buttonElement.blur();
		}
	}

	/**
	 * Toggle pressed state (for toggle buttons)
	 */
	function toggle() {
		state.pressed = !state.pressed;
	}

	/**
	 * Set disabled state
	 */
	function setDisabled(disabled: boolean) {
		state.disabled = disabled;
	}

	/**
	 * Set loading state
	 */
	function setLoading(loading: boolean) {
		state.loading = loading;
	}

	/**
	 * Set pressed state
	 */
	function setPressed(pressed: boolean) {
		state.pressed = pressed;
	}

	/**
	 * Clean up resources
	 */
	function destroy() {
		if (buttonElement) {
			const node = buttonElement;
			node.removeEventListener('click', handleClick);
			node.removeEventListener('keydown', handleKeyDown);
			node.removeEventListener('focus', handleFocus);
			node.removeEventListener('blur', handleBlur);
			buttonElement = null;
		}
		onDestroy?.();
	}

	return {
		state,
		actions: {
			button,
		},
		helpers: {
			click,
			focus,
			blur,
			toggle,
			setDisabled,
			setLoading,
			setPressed,
			destroy,
		},
	};
}

/**
 * Type guard to check if an element is a button
 */
export function isButton(
	element: unknown
): element is HTMLButtonElement | HTMLInputElement | HTMLAnchorElement {
	if (element instanceof HTMLButtonElement) return true;
	if (element instanceof HTMLInputElement) {
		const type = (element as HTMLInputElement).type;
		return type === 'button' || type === 'submit' || type === 'reset';
	}
	if (element instanceof HTMLAnchorElement) {
		return element.getAttribute('role') === 'button';
	}
	return false;
}
