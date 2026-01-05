/**
 * Headless Alert Primitive
 *
 * Provides all alert behavior without any styling opinions.
 * Handles ARIA attributes, keyboard navigation, and visibility state.
 *
 * @example
 * ```svelte
 * <script>
 *   import { createAlert } from 'src/lib/greater/headless/alert';
 *
 *   const alert = createAlert({
 *     variant: 'error',
 *     dismissible: true,
 *     onDismiss: () => console.log('dismissed')
 *   });
 * </script>
 *
 * <div use:alert.actions.container class="my-custom-alert">
 *   <div use:alert.actions.icon>Icon</div>
 *   <div>Alert content</div>
 *   <button use:alert.actions.dismiss>Close</button>
 * </div>
 * ```
 *
 * @module @equaltoai/greater-components-headless/alert
 */

import type { ActionReturn, BaseBuilderConfig } from '../types/common.js';
import { generateId } from '../utils/id.js';
import { isEscapeKey } from '../utils/keyboard.js';

/**
 * Alert variant type
 */
export type AlertVariant = 'error' | 'warning' | 'success' | 'info';

/**
 * Alert configuration options
 */
export interface AlertConfig extends BaseBuilderConfig {
	/**
	 * Alert variant determining ARIA role
	 * - 'error' | 'warning': role="alert" with aria-live="assertive"
	 * - 'success' | 'info': role="status" with aria-live="polite"
	 * @defaultValue 'info'
	 */
	variant?: AlertVariant;

	/**
	 * Whether the alert can be dismissed
	 * @defaultValue false
	 */
	dismissible?: boolean;

	/**
	 * Initial visibility state
	 * @defaultValue true
	 */
	visible?: boolean;

	/**
	 * Callback when alert is dismissed
	 */
	onDismiss?: () => void;

	/**
	 * Callback when action button is clicked
	 */
	onAction?: () => void;

	/**
	 * Keyboard event handler
	 */
	onKeyDown?: (event: KeyboardEvent) => void;
}

/**
 * Alert state
 */
export interface AlertState {
	/**
	 * Alert variant
	 */
	variant: AlertVariant;

	/**
	 * Whether the alert is visible
	 */
	visible: boolean;

	/**
	 * Whether the alert is dismissible
	 */
	dismissible: boolean;

	/**
	 * The alert's ID
	 */
	id: string;

	/**
	 * ARIA role based on variant
	 */
	role: 'alert' | 'status';

	/**
	 * ARIA live region setting
	 */
	ariaLive: 'assertive' | 'polite';
}

/**
 * Alert builder return type
 */
export interface AlertBuilder {
	/**
	 * Current alert state (reactive with Svelte 5 runes)
	 */
	state: AlertState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: {
		/**
		 * Main container action - apply to alert wrapper element
		 */
		container: (node: HTMLElement) => ActionReturn;

		/**
		 * Icon action - apply to icon container
		 */
		icon: (node: HTMLElement) => ActionReturn;

		/**
		 * Dismiss button action - apply to dismiss button
		 */
		dismiss: (node: HTMLButtonElement) => ActionReturn;

		/**
		 * Action button action - apply to action button
		 */
		action: (node: HTMLButtonElement) => ActionReturn;
	};

	/**
	 * Helper methods
	 */
	helpers: {
		/**
		 * Programmatically dismiss the alert
		 */
		dismiss: () => void;

		/**
		 * Programmatically show the alert
		 */
		show: () => void;

		/**
		 * Toggle alert visibility
		 */
		toggle: () => void;

		/**
		 * Set visibility state
		 */
		setVisible: (visible: boolean) => void;

		/**
		 * Clean up resources
		 */
		destroy: () => void;
	};
}

/**
 * Create a headless alert
 *
 * @param config - Alert configuration
 * @returns Alert builder with state, actions, and helpers
 *
 * @public
 */
export function createAlert(config: AlertConfig = {}): AlertBuilder {
	const {
		variant: initialVariant = 'info',
		dismissible: initialDismissible = false,
		visible: initialVisible = true,
		id: customId,
		onDismiss,
		onAction,
		onKeyDown,
		onDestroy,
	} = config;

	let containerElement: HTMLElement | null = null;

	// Compute ARIA attributes based on variant
	function getAriaRole(v: AlertVariant): 'alert' | 'status' {
		return v === 'error' || v === 'warning' ? 'alert' : 'status';
	}

	function getAriaLive(v: AlertVariant): 'assertive' | 'polite' {
		return v === 'error' || v === 'warning' ? 'assertive' : 'polite';
	}

	// Reactive state
	const internalState: AlertState = {
		variant: initialVariant,
		visible: initialVisible,
		dismissible: initialDismissible,
		id: customId || generateId('alert'),
		role: getAriaRole(initialVariant),
		ariaLive: getAriaLive(initialVariant),
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof AlertState, value) {
			target[prop] = value as never;
			if (prop === 'variant') {
				target.role = getAriaRole(value as AlertVariant);
				target.ariaLive = getAriaLive(value as AlertVariant);
			}
			updateDOM();
			return true;
		},
	});

	/**
	 * Update DOM when state changes
	 */
	function updateDOM() {
		if (containerElement) {
			updateAriaAttributes(containerElement);
		}
	}

	/**
	 * Handle keyboard events
	 */
	function handleKeyDown(event: KeyboardEvent) {
		if (state.dismissible && isEscapeKey(event)) {
			event.preventDefault();
			handleDismiss();
		}
		onKeyDown?.(event);
	}

	/**
	 * Handle dismiss
	 */
	function handleDismiss() {
		if (!state.dismissible) return;
		state.visible = false;
		onDismiss?.();
	}

	/**
	 * Handle action
	 */
	function handleAction() {
		onAction?.();
	}

	/**
	 * Container action
	 */
	function container(node: HTMLElement): ActionReturn {
		containerElement = node;

		// Set initial attributes
		node.id = state.id;
		updateAriaAttributes(node);

		// Add event listeners
		node.addEventListener('keydown', handleKeyDown);

		return {
			update() {
				updateAriaAttributes(node);
			},
			destroy() {
				node.removeEventListener('keydown', handleKeyDown);
				containerElement = null;
				onDestroy?.();
			},
		};
	}

	/**
	 * Icon action
	 */
	function icon(node: HTMLElement): ActionReturn {
		node.setAttribute('aria-hidden', 'true');

		return {
			destroy() {},
		};
	}

	/**
	 * Dismiss button action
	 */
	function dismiss(node: HTMLButtonElement): ActionReturn {
		node.setAttribute('type', 'button');
		node.setAttribute('aria-label', 'Dismiss alert');

		function handleClick(event: MouseEvent) {
			event.preventDefault();
			handleDismiss();
		}

		node.addEventListener('click', handleClick);

		return {
			destroy() {
				node.removeEventListener('click', handleClick);
			},
		};
	}

	/**
	 * Action button action
	 */
	function action(node: HTMLButtonElement): ActionReturn {
		node.setAttribute('type', 'button');

		function handleClick(event: MouseEvent) {
			event.preventDefault();
			handleAction();
		}

		node.addEventListener('click', handleClick);

		return {
			destroy() {
				node.removeEventListener('click', handleClick);
			},
		};
	}

	/**
	 * Update ARIA attributes based on state
	 */
	function updateAriaAttributes(node: HTMLElement) {
		node.setAttribute('role', state.role);
		node.setAttribute('aria-live', state.ariaLive);
	}

	return {
		state,
		actions: {
			container,
			icon,
			dismiss,
			action,
		},
		helpers: {
			dismiss: handleDismiss,
			show: () => {
				state.visible = true;
			},
			toggle: () => {
				state.visible = !state.visible;
			},
			setVisible: (visible: boolean) => {
				state.visible = visible;
			},
			destroy: () => {
				containerElement = null;
				onDestroy?.();
			},
		},
	};
}

/**
 * Type guard to check if a value is an AlertVariant
 */
export function isAlertVariant(value: unknown): value is AlertVariant {
	return typeof value === 'string' && ['error', 'warning', 'success', 'info'].includes(value);
}
