/**
 * Focus Trap Behavior
 *
 * Framework-agnostic utility for trapping focus within a container element.
 * Used by modals, dialogs, and other overlay components.
 *
 * @module @equaltoai/greater-components-headless/behaviors/focus-trap
 */

/**
 * Focus trap configuration
 */
export interface FocusTrapConfig {
	/**
	 * Element to focus when trap is activated
	 * Can be an element, selector string, or function returning element
	 */
	initialFocus?: HTMLElement | string | (() => HTMLElement | null);

	/**
	 * Element to return focus to when trap is deactivated
	 * Defaults to previously focused element
	 */
	returnFocusTo?: HTMLElement | (() => HTMLElement | null);

	/**
	 * Whether to return focus when deactivated
	 * @default true
	 */
	returnFocus?: boolean;

	/**
	 * Whether to auto-focus first focusable element if initialFocus not specified
	 * @default true
	 */
	autoFocus?: boolean;

	/**
	 * Callback when focus escapes the trap (for edge cases)
	 */
	onFocusEscape?: () => void;

	/**
	 * Callback when trap is activated
	 */
	onActivate?: () => void;

	/**
	 * Callback when trap is deactivated
	 */
	onDeactivate?: () => void;
}

/**
 * Focus trap state
 */
export interface FocusTrapState {
	/**
	 * Whether the trap is currently active
	 */
	active: boolean;

	/**
	 * The container element being trapped
	 */
	container: HTMLElement | null;

	/**
	 * Element that was focused before trap activation
	 */
	previouslyFocused: HTMLElement | null;
}

/**
 * Focus trap return type
 */
export interface FocusTrap {
	/**
	 * Current state
	 */
	state: FocusTrapState;

	/**
	 * Activate the focus trap on a container
	 */
	activate: (container: HTMLElement) => void;

	/**
	 * Deactivate the focus trap
	 */
	deactivate: () => void;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<FocusTrapConfig>) => void;

	/**
	 * Destroy the trap and clean up
	 */
	destroy: () => void;
}

/**
 * Focusable element selectors
 */
const FOCUSABLE_SELECTORS = [
	'a[href]',
	'button:not([disabled])',
	'textarea:not([disabled])',
	'input:not([disabled]):not([type="hidden"])',
	'select:not([disabled])',
	'[tabindex]:not([tabindex="-1"])',
	'[contenteditable="true"]',
	'audio[controls]',
	'video[controls]',
	'details > summary:first-of-type',
].join(', ');

/**
 * Get all focusable elements within a container
 */
export function getFocusableElements(container: HTMLElement): HTMLElement[] {
	const elements = Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS));

	return elements.filter((el) => {
		const style = window.getComputedStyle(el);

		// Check if element is visible
		if (el.offsetParent === null && style.position !== 'fixed') {
			return false;
		}

		// Check computed visibility
		if (style.visibility === 'hidden' || style.display === 'none') {
			return false;
		}

		return true;
	});
}

/**
 * Get first focusable element in container
 */
export function getFirstFocusable(container: HTMLElement): HTMLElement | null {
	const focusable = getFocusableElements(container);
	return focusable[0] || null;
}

/**
 * Get last focusable element in container
 */
export function getLastFocusable(container: HTMLElement): HTMLElement | null {
	const focusable = getFocusableElements(container);
	return focusable[focusable.length - 1] || null;
}

/**
 * Check if element is focusable
 */
export function isFocusable(element: HTMLElement): boolean {
	return element.matches(FOCUSABLE_SELECTORS);
}

/**
 * Create a focus trap
 */
export function createFocusTrap(config: FocusTrapConfig = {}): FocusTrap {
	let currentConfig = { ...config };

	const state: FocusTrapState = {
		active: false,
		container: null,
		previouslyFocused: null,
	};

	let keydownHandler: ((event: KeyboardEvent) => void) | null = null;
	let focusinHandler: ((event: FocusEvent) => void) | null = null;

	/**
	 * Resolve initial focus element
	 */
	function resolveInitialFocus(container: HTMLElement): HTMLElement | null {
		const { initialFocus, autoFocus = true } = currentConfig;

		if (initialFocus) {
			if (typeof initialFocus === 'function') {
				return initialFocus();
			}
			if (typeof initialFocus === 'string') {
				return container.querySelector<HTMLElement>(initialFocus);
			}
			return initialFocus;
		}

		if (autoFocus) {
			return getFirstFocusable(container);
		}

		return null;
	}

	/**
	 * Handle Tab key navigation
	 */
	function handleKeyDown(event: KeyboardEvent): void {
		if (!state.active || !state.container) return;
		if (event.key !== 'Tab') return;

		const focusable = getFocusableElements(state.container);
		if (focusable.length === 0) {
			event.preventDefault();
			return;
		}

		const firstFocusable = focusable[0];
		const lastFocusable = focusable[focusable.length - 1];
		const activeElement = document.activeElement as HTMLElement;

		if (event.shiftKey) {
			// Shift+Tab: wrap to last if at first
			if (activeElement === firstFocusable || !state.container.contains(activeElement)) {
				event.preventDefault();
				lastFocusable?.focus();
			}
		} else {
			// Tab: wrap to first if at last
			if (activeElement === lastFocusable || !state.container.contains(activeElement)) {
				event.preventDefault();
				firstFocusable?.focus();
			}
		}
	}

	/**
	 * Handle focus escaping the container
	 */
	function handleFocusIn(event: FocusEvent): void {
		if (!state.active || !state.container) return;

		const target = event.target as HTMLElement;

		// If focus moved outside container, bring it back
		if (!state.container.contains(target)) {
			event.preventDefault();
			event.stopPropagation();

			const focusable = getFocusableElements(state.container);
			if (focusable.length > 0) {
				focusable[0]?.focus();
			}

			currentConfig.onFocusEscape?.();
		}
	}

	/**
	 * Activate the focus trap
	 */
	function activate(container: HTMLElement): void {
		if (state.active) {
			deactivate();
		}

		state.container = container;
		state.previouslyFocused = document.activeElement as HTMLElement;
		state.active = true;

		// Set up event listeners
		keydownHandler = handleKeyDown;
		focusinHandler = handleFocusIn;

		document.addEventListener('keydown', keydownHandler, true);
		document.addEventListener('focusin', focusinHandler, true);

		// Focus initial element
		const initialElement = resolveInitialFocus(container);
		if (initialElement) {
			// Use setTimeout to ensure DOM is ready
			setTimeout(() => {
				initialElement.focus();
			}, 0);
		}

		currentConfig.onActivate?.();
	}

	/**
	 * Deactivate the focus trap
	 */
	function deactivate(): void {
		if (!state.active) return;

		// Remove event listeners
		if (keydownHandler) {
			document.removeEventListener('keydown', keydownHandler, true);
			keydownHandler = null;
		}
		if (focusinHandler) {
			document.removeEventListener('focusin', focusinHandler, true);
			focusinHandler = null;
		}

		// Return focus
		const { returnFocus = true, returnFocusTo } = currentConfig;
		if (returnFocus) {
			const returnTarget =
				typeof returnFocusTo === 'function'
					? returnFocusTo()
					: returnFocusTo || state.previouslyFocused;

			if (returnTarget && typeof returnTarget.focus === 'function') {
				returnTarget.focus();
			}
		}

		state.active = false;
		state.container = null;

		currentConfig.onDeactivate?.();
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<FocusTrapConfig>): void {
		currentConfig = { ...currentConfig, ...newConfig };
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		deactivate();
		state.previouslyFocused = null;
	}

	return {
		state,
		activate,
		deactivate,
		updateConfig,
		destroy,
	};
}
