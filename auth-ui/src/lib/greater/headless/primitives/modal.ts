/**
 * Headless Modal Primitive
 *
 * Provides accessible modal/dialog behavior without any styling.
 * Features:
 * - Focus trapping
 * - ESC to close
 * - Click outside to close (optional)
 * - Body scroll locking
 * - ARIA attributes
 * - Portal rendering (optional)
 * - Animation support
 *
 * @module @equaltoai/greater-components-headless/modal
 */

import { generateId } from '../utils/id';
import type { Action } from '../types/common';

/**
 * Modal state
 */
export interface ModalState {
	/**
	 * Whether modal is open
	 */
	open: boolean;

	/**
	 * Modal ID
	 */
	id: string;

	/**
	 * Whether modal is animating
	 */
	animating: boolean;

	/**
	 * Whether modal has backdrop
	 */
	hasBackdrop: boolean;

	/**
	 * Whether to prevent body scroll
	 */
	preventScroll: boolean;

	/**
	 * Whether to close on ESC key
	 */
	closeOnEscape: boolean;

	/**
	 * Whether to close on backdrop click
	 */
	closeOnBackdrop: boolean;

	/**
	 * Whether to return focus on close
	 */
	returnFocus: boolean;

	/**
	 * Whether modal content is mounted in DOM
	 */
	mounted: boolean;
}

/**
 * Modal configuration
 */
export interface ModalConfig {
	/**
	 * Initial/controlled open state
	 */
	open?: boolean;

	/**
	 * Custom modal ID
	 */
	id?: string;

	/**
	 * Whether modal has backdrop
	 */
	hasBackdrop?: boolean;

	/**
	 * Prevent body scroll when open
	 */
	preventScroll?: boolean;

	/**
	 * Close on ESC key
	 */
	closeOnEscape?: boolean;

	/**
	 * Close on backdrop click
	 */
	closeOnBackdrop?: boolean;

	/**
	 * Return focus to trigger on close
	 */
	returnFocus?: boolean;

	/**
	 * Trap focus within modal
	 */
	trapFocus?: boolean;

	/**
	 * Element to focus when modal opens (defaults to first focusable)
	 */
	initialFocus?: HTMLElement | (() => HTMLElement | null);

	/**
	 * ARIA labelledby attribute
	 */
	labelledBy?: string;

	/**
	 * ARIA describedby attribute
	 */
	describedBy?: string;

	/**
	 * Called when open state changes
	 */
	onOpenChange?: (open: boolean) => void;

	/**
	 * Called when modal opens
	 */
	onOpen?: () => void;

	/**
	 * Called when modal closes
	 */
	onClose?: () => void;

	/**
	 * Called before modal closes (can prevent close by returning false)
	 */
	onBeforeClose?: () => boolean | Promise<boolean>;

	/**
	 * Called on destroy
	 */
	onDestroy?: () => void;
}

/**
 * Modal actions
 */
export interface ModalActions {
	/**
	 * Action for modal backdrop
	 */
	backdrop: Action<HTMLElement>;

	/**
	 * Action for modal content
	 */
	content: Action<HTMLElement>;

	/**
	 * Action for trigger button
	 */
	trigger: Action<HTMLButtonElement>;

	/**
	 * Action for close button
	 */
	close: Action<HTMLButtonElement>;
}

/**
 * Modal helpers
 */
export interface ModalHelpers {
	/**
	 * Open the modal
	 */
	open: () => void;

	/**
	 * Close the modal
	 */
	close: () => void;

	/**
	 * Toggle the modal
	 */
	toggle: () => void;
}

/**
 * Get all focusable elements within a container
 */
function getFocusableElements(container: HTMLElement): HTMLElement[] {
	const focusableSelectors = [
		'a[href]',
		'button:not([disabled])',
		'textarea:not([disabled])',
		'input:not([disabled])',
		'select:not([disabled])',
		'[tabindex]:not([tabindex="-1"])',
	].join(', ');

	return Array.from(container.querySelectorAll<HTMLElement>(focusableSelectors));
}

/**
 * Create a headless modal primitive
 */
export function createModal(config: ModalConfig = {}) {
	const {
		open: initialOpen = false,
		id: customId,
		hasBackdrop = true,
		preventScroll = true,
		closeOnEscape: closeOnEscapeProp = true,
		closeOnBackdrop: closeOnBackdropProp = true,
		returnFocus: shouldReturnFocus = true,
		trapFocus = true,
		initialFocus,
		labelledBy,
		describedBy,
		onOpen,
		onClose,
		onOpenChange,
		onBeforeClose,
		onDestroy,
	} = config;

	// Reactive state
	const internalState: ModalState = {
		open: initialOpen,
		id: customId || generateId('modal'),
		animating: false,
		hasBackdrop,
		preventScroll,
		closeOnEscape: closeOnEscapeProp,
		closeOnBackdrop: closeOnBackdropProp,
		returnFocus: shouldReturnFocus,
		mounted: false,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof ModalState, value) {
			const oldValue = target[prop];
			target[prop] = value as never;
			if (prop === 'open' && oldValue !== value) {
				onOpenChange?.(value as boolean);
			}
			return true;
		},
	});

	let contentElement: HTMLElement | null = null;
	let triggerElement: HTMLButtonElement | null = null;
	let previouslyFocusedElement: HTMLElement | null = null;
	let hasScrollLock = false;

	const SCROLL_LOCK_CLASS = 'gr-scroll-locked';
	const SCROLL_LOCK_COUNT_ATTR = 'data-gr-scroll-lock-count';

	function lockBodyScroll(): void {
		const current = Number(document.body.getAttribute(SCROLL_LOCK_COUNT_ATTR) ?? '0');
		const next = current + 1;
		document.body.setAttribute(SCROLL_LOCK_COUNT_ATTR, String(next));
		document.body.classList.add(SCROLL_LOCK_CLASS);
		hasScrollLock = true;
	}

	function unlockBodyScroll(): void {
		if (!hasScrollLock) return;

		const current = Number(document.body.getAttribute(SCROLL_LOCK_COUNT_ATTR) ?? '0');
		const next = Math.max(0, current - 1);

		if (next === 0) {
			document.body.removeAttribute(SCROLL_LOCK_COUNT_ATTR);
			document.body.classList.remove(SCROLL_LOCK_CLASS);
		} else {
			document.body.setAttribute(SCROLL_LOCK_COUNT_ATTR, String(next));
		}

		hasScrollLock = false;
	}

	// Initialize if opened
	if (initialOpen) {
		previouslyFocusedElement = document.activeElement as HTMLElement;
		if (preventScroll) {
			lockBodyScroll();
		}
	}

	/**
	 * Open the modal
	 */
	function open() {
		if (state.open) return;

		state.animating = true;
		state.open = true;

		// Store previously focused element
		previouslyFocusedElement = document.activeElement as HTMLElement;

		// Lock body scroll
		if (state.preventScroll) {
			lockBodyScroll();
		}

		// Focus initial element
		// Use setTimeout only if content element is not yet available
		if (contentElement) {
			const focusTarget =
				typeof initialFocus === 'function'
					? initialFocus()
					: initialFocus || getFocusableElements(contentElement)[0];

			if (focusTarget) {
				focusTarget.focus();
				// Manually trigger focus event for JSDOM compatibility
				focusTarget.dispatchEvent(new FocusEvent('focus', { bubbles: false }));
			}
			state.animating = false;
		} else {
			// Defer if content not yet mounted
			setTimeout(() => {
				if (contentElement) {
					const focusTarget =
						typeof initialFocus === 'function'
							? initialFocus()
							: initialFocus || getFocusableElements(contentElement)[0];

					if (focusTarget) {
						focusTarget.focus();
						focusTarget.dispatchEvent(new FocusEvent('focus', { bubbles: false }));
					}
				}
				state.animating = false;
			}, 0);
		}

		onOpen?.();
	}

	/**
	 * Close the modal
	 */
	function close() {
		if (!state.open) return;

		// Check if close should be prevented (synchronous only for simplicity)
		if (onBeforeClose) {
			const shouldClose = onBeforeClose();
			// Handle both sync and async returns
			if (shouldClose instanceof Promise) {
				shouldClose.then((result) => {
					if (result) performClose();
				});
				return;
			} else if (!shouldClose) {
				return;
			}
		}

		performClose();
	}

	/**
	 * Perform the actual close operation
	 */
	function performClose() {
		state.animating = true;
		state.open = false;

		// Restore body scroll
		if (state.preventScroll) {
			unlockBodyScroll();
		}

		// Return focus (synchronous for better test compatibility)
		if (state.returnFocus) {
			const returnTarget = previouslyFocusedElement || triggerElement;
			if (returnTarget) {
				returnTarget.focus();
				// Manually trigger focus event for JSDOM compatibility
				returnTarget.dispatchEvent(new FocusEvent('focus', { bubbles: false }));
			}
		}

		state.animating = false;
		onClose?.();
	}

	/**
	 * Toggle the modal
	 */
	function toggle() {
		if (state.open) {
			close();
		} else {
			open();
		}
	}

	/**
	 * Handle ESC key
	 */
	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Escape' && state.closeOnEscape && state.open) {
			event.preventDefault();
			close();
		}

		// Trap focus
		if (event.key === 'Tab' && trapFocus && contentElement) {
			const focusableElements = getFocusableElements(contentElement);
			if (focusableElements.length === 0) return;

			const firstFocusable = focusableElements[0];
			const lastFocusable = focusableElements[focusableElements.length - 1];

			if (event.shiftKey) {
				// Shift+Tab
				if (document.activeElement === firstFocusable) {
					event.preventDefault();
					lastFocusable?.focus();
				}
			} else {
				// Tab
				if (document.activeElement === lastFocusable) {
					event.preventDefault();
					firstFocusable?.focus();
				}
			}
		}
	}

	/**
	 * Backdrop action
	 */
	const backdrop: Action<HTMLElement> = (node: HTMLElement) => {
		function handleClick(event: MouseEvent) {
			if (state.closeOnBackdrop && event.target === node) {
				close();
			}
		}

		node.addEventListener('click', handleClick);
		document.addEventListener('keydown', handleKeyDown);

		// Set ARIA attributes
		node.setAttribute('aria-modal', 'true');
		node.setAttribute('data-modal-backdrop', '');

		return {
			destroy() {
				node.removeEventListener('click', handleClick);
				document.removeEventListener('keydown', handleKeyDown);
				onDestroy?.();
			},
		};
	};

	/**
	 * Content action
	 */
	const content: Action<HTMLElement> = (node: HTMLElement) => {
		contentElement = node;
		state.mounted = true;

		// Attach keyboard handler to content as well (in case no backdrop)
		node.addEventListener('keydown', handleKeyDown);

		// Set ARIA attributes
		node.setAttribute('role', 'dialog');
		node.setAttribute('aria-modal', 'true');
		node.id = state.id;

		// Set labelledby and describedby if provided
		if (labelledBy) {
			node.setAttribute('aria-labelledby', labelledBy);
		} else {
			node.setAttribute('aria-labelledby', `${state.id}-title`);
		}

		if (describedBy) {
			node.setAttribute('aria-describedby', describedBy);
		} else {
			node.setAttribute('aria-describedby', `${state.id}-description`);
		}

		// Make focusable for initial focus
		if (!node.tabIndex || node.tabIndex < 0) {
			node.tabIndex = 0;
		}

		return {
			destroy() {
				node.removeEventListener('keydown', handleKeyDown);
				state.mounted = false;
				contentElement = null;

				// Restore body overflow if modal was open
				if (state.open && state.preventScroll) {
					unlockBodyScroll();
				}

				onDestroy?.();
			},
		};
	};

	/**
	 * Trigger action
	 */
	const trigger: Action<HTMLButtonElement> = (node: HTMLButtonElement) => {
		triggerElement = node;

		function handleClick() {
			open();
		}

		node.addEventListener('click', handleClick);
		node.setAttribute('aria-haspopup', 'dialog');
		node.setAttribute('aria-expanded', String(state.open));
		node.setAttribute('aria-controls', state.id);

		return {
			update() {
				node.setAttribute('aria-expanded', String(state.open));
			},
			destroy() {
				node.removeEventListener('click', handleClick);
				triggerElement = null;
			},
		};
	};

	/**
	 * Close button action
	 */
	const closeButton: Action<HTMLButtonElement> = (node: HTMLButtonElement) => {
		function handleClick() {
			close();
		}

		node.addEventListener('click', handleClick);
		node.setAttribute('aria-label', 'Close');

		return {
			destroy() {
				node.removeEventListener('click', handleClick);
			},
		};
	};

	const actions: ModalActions = {
		backdrop,
		content,
		trigger,
		close: closeButton,
	};

	const helpers: ModalHelpers = {
		open,
		close,
		toggle,
	};

	return {
		state,
		actions,
		helpers,
	};
}
