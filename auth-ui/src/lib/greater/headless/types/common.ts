/**
 * Common types for headless UI primitives
 * @module @equaltoai/greater-components-headless/types
 */

/**
 * Base builder configuration shared by all primitives
 */
export interface BaseBuilderConfig {
	/**
	 * Whether the element is disabled
	 * @defaultValue false
	 */
	disabled?: boolean;

	/**
	 * Custom ID for the element (auto-generated if not provided)
	 */
	id?: string;

	/**
	 * Callback when the element is destroyed
	 */
	onDestroy?: () => void;
}

/**
 * Action return type for Svelte use:action directive
 */
export interface ActionReturn<P = never> {
	update?: (parameters: P) => void;
	destroy?: () => void;
}

/**
 * Svelte action type
 * A function that receives a DOM node and returns an action object
 */
export type Action<T extends HTMLElement = HTMLElement, P = never> = (
	node: T,
	parameters?: P
) => ActionReturn<P> | void;

/**
 * Builder return type - provides actions and state
 */
export interface Builder<TState = unknown, TActions = unknown> {
	/**
	 * Reactive state that can be accessed and modified
	 */
	state: TState;

	/**
	 * Actions (Svelte actions) to apply to DOM elements
	 */
	actions: TActions;

	/**
	 * Helper functions for common operations
	 */
	helpers: {
		/**
		 * Clean up all resources
		 */
		destroy: () => void;
	};
}

/**
 * Element attributes that can be applied to any HTML element
 */
export interface ElementAttributes {
	id?: string;
	role?: string;
	'aria-label'?: string;
	'aria-labelledby'?: string;
	'aria-describedby'?: string;
	'aria-disabled'?: boolean | 'true' | 'false';
	'aria-expanded'?: boolean | 'true' | 'false';
	'aria-haspopup'?: boolean | 'true' | 'false' | 'menu' | 'listbox' | 'tree' | 'grid' | 'dialog';
	'aria-controls'?: string;
	'aria-current'?: boolean | 'true' | 'false' | 'page' | 'step' | 'location' | 'date' | 'time';
	'aria-hidden'?: boolean | 'true' | 'false';
	'aria-live'?: 'off' | 'polite' | 'assertive';
	'aria-selected'?: boolean | 'true' | 'false';
	'aria-pressed'?: boolean | 'true' | 'false' | 'mixed';
	tabindex?: number;
	[key: string]: unknown;
}

/**
 * Keyboard event handler utilities
 */
export type KeyboardEventHandler = (event: KeyboardEvent) => void;

/**
 * Mouse event handler utilities
 */
export type MouseEventHandler = (event: MouseEvent) => void;

/**
 * Focus event handler utilities
 */
export type FocusEventHandler = (event: FocusEvent) => void;

/**
 * Generic event handler
 */
export type EventHandler<T extends Event = Event> = (event: T) => void;
