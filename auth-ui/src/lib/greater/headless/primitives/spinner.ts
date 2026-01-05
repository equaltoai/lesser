/**
 * Spinner Primitive
 *
 * Headless primitive for loading spinner states with animation control.
 * Provides behavior for displaying loading indicators with various sizes and colors.
 *
 * @module primitives/spinner
 */

import type { BaseBuilderConfig } from '../types/common.js';
import { generateId } from '../utils/id.js';

/**
 * Spinner size options
 */
export type SpinnerSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

/**
 * Spinner color options
 */
export type SpinnerColor = 'primary' | 'current' | 'white' | 'gray';

/**
 * Spinner configuration
 */
export interface SpinnerConfig extends BaseBuilderConfig {
	/**
	 * Size of the spinner
	 * @defaultValue 'md'
	 */
	size?: SpinnerSize;

	/**
	 * Color variant of the spinner
	 * @defaultValue 'primary'
	 */
	color?: SpinnerColor;

	/**
	 * Accessible label for screen readers
	 * @defaultValue 'Loading'
	 */
	label?: string;

	/**
	 * Whether the spinner is currently active/visible
	 * @defaultValue true
	 */
	active?: boolean;

	/**
	 * Custom class name
	 */
	class?: string;

	/**
	 * Callback when spinner becomes active
	 */
	onStart?: () => void;

	/**
	 * Callback when spinner becomes inactive
	 */
	onStop?: () => void;
}

/**
 * Spinner state
 */
export interface SpinnerState {
	/**
	 * Unique ID for the spinner
	 */
	id: string;

	/**
	 * Size of the spinner
	 */
	size: SpinnerSize;

	/**
	 * Color variant
	 */
	color: SpinnerColor;

	/**
	 * Accessible label
	 */
	label: string;

	/**
	 * Whether the spinner is active
	 */
	active: boolean;

	/**
	 * Whether the spinner is disabled
	 */
	disabled: boolean;

	/**
	 * Custom class name
	 */
	class: string;
}

/**
 * Spinner actions
 */
export interface SpinnerActions {
	/**
	 * Svelte action for spinner element
	 */
	spinner: (node: HTMLElement) => { destroy: () => void };
}

/**
 * Spinner helpers
 */
export interface SpinnerHelpers {
	/**
	 * Start the spinner animation
	 */
	start: () => void;

	/**
	 * Stop the spinner animation
	 */
	stop: () => void;

	/**
	 * Toggle the spinner state
	 */
	toggle: () => void;

	/**
	 * Set the active state
	 */
	setActive: (active: boolean) => void;

	/**
	 * Set the size
	 */
	setSize: (size: SpinnerSize) => void;

	/**
	 * Set the color
	 */
	setColor: (color: SpinnerColor) => void;

	/**
	 * Set the label
	 */
	setLabel: (label: string) => void;

	/**
	 * Clean up resources
	 */
	destroy: () => void;
}

/**
 * Spinner builder return type
 */
export interface Spinner {
	/**
	 * Current state
	 */
	state: SpinnerState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: SpinnerActions;

	/**
	 * Helper functions
	 */
	helpers: SpinnerHelpers;
}

/**
 * Create a headless spinner
 *
 * @param config - Spinner configuration
 * @returns Spinner builder with state, actions, and helpers
 *
 * @example
 * ```svelte
 * <script>
 *   import { createSpinner } from 'src/lib/greater/headless';
 *
 *   const spinner = createSpinner({
 *     size: 'md',
 *     label: 'Loading data...'
 *   });
 * </script>
 *
 * <span use:spinner.actions.spinner class="my-spinner">
 *   <!-- SVG spinner icon -->
 * </span>
 * ```
 *
 * @public
 */
export function createSpinner(config: SpinnerConfig = {}): Spinner {
	const {
		id: customId,
		size: initialSize = 'md',
		color: initialColor = 'primary',
		label: initialLabel = 'Loading',
		active: initialActive = true,
		disabled: initialDisabled = false,
		class: initialClass = '',
		onStart,
		onStop,
		onDestroy,
	} = config;

	// Generate unique ID
	const id = customId || generateId('spinner');

	// Internal state
	let spinnerElement: HTMLElement | null = null;

	// Mutable state object
	const internalState: SpinnerState = {
		id,
		size: initialSize,
		color: initialColor,
		label: initialLabel,
		active: initialActive,
		disabled: initialDisabled,
		class: initialClass,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof SpinnerState, value) {
			const oldValue = target[prop];
			target[prop] = value as never;

			// Handle active state changes
			if (prop === 'active' && oldValue !== value) {
				if (value) {
					onStart?.();
				} else {
					onStop?.();
				}
			}

			updateDOM();
			return true;
		},
	});

	/**
	 * Update DOM attributes based on state
	 */
	function updateDOM() {
		if (!spinnerElement) return;

		spinnerElement.setAttribute('role', 'status');
		spinnerElement.setAttribute('aria-label', state.label);
		spinnerElement.setAttribute('data-size', state.size);
		spinnerElement.setAttribute('data-color', state.color);
		spinnerElement.setAttribute('data-active', String(state.active));

		if (state.disabled) {
			spinnerElement.setAttribute('aria-disabled', 'true');
		} else {
			spinnerElement.removeAttribute('aria-disabled');
		}
	}

	/**
	 * Spinner action
	 */
	function spinner(node: HTMLElement): { destroy: () => void } {
		spinnerElement = node;

		// Set initial attributes
		node.id = state.id;
		updateDOM();

		return {
			destroy() {
				spinnerElement = null;
				onDestroy?.();
			},
		};
	}

	/**
	 * Start the spinner
	 */
	function start() {
		state.active = true;
	}

	/**
	 * Stop the spinner
	 */
	function stop() {
		state.active = false;
	}

	/**
	 * Toggle the spinner
	 */
	function toggle() {
		state.active = !state.active;
	}

	/**
	 * Set active state
	 */
	function setActive(active: boolean) {
		state.active = active;
	}

	/**
	 * Set size
	 */
	function setSize(size: SpinnerSize) {
		state.size = size;
	}

	/**
	 * Set color
	 */
	function setColor(color: SpinnerColor) {
		state.color = color;
	}

	/**
	 * Set label
	 */
	function setLabel(label: string) {
		state.label = label;
	}

	/**
	 * Destroy and clean up
	 */
	function destroy() {
		spinnerElement = null;
		onDestroy?.();
	}

	return {
		state,
		actions: {
			spinner,
		},
		helpers: {
			start,
			stop,
			toggle,
			setActive,
			setSize,
			setColor,
			setLabel,
			destroy,
		},
	};
}
