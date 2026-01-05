/**
 * Headless Tooltip Primitive
 *
 * Provides accessible tooltip behavior without any styling.
 * Features:
 * - Smart positioning (avoids viewport edges)
 * - Hover and focus triggers
 * - Keyboard accessible
 * - ARIA attributes
 * - Configurable delays
 * - Portal rendering support
 *
 * @module @equaltoai/greater-components-headless/tooltip
 */

import { generateId } from '../utils/id';
import type { Action } from '../types/common';

/**
 * Tooltip placement options
 */
export type TooltipPlacement =
	| 'top'
	| 'top-start'
	| 'top-end'
	| 'bottom'
	| 'bottom-start'
	| 'bottom-end'
	| 'left'
	| 'left-start'
	| 'left-end'
	| 'right'
	| 'right-start'
	| 'right-end';

/**
 * Tooltip state
 */
export interface TooltipState {
	/**
	 * Whether tooltip is visible
	 */
	open: boolean;

	/**
	 * Tooltip ID
	 */
	id: string;

	/**
	 * Current placement (may differ from preferred due to smart positioning)
	 */
	placement: TooltipPlacement;

	/**
	 * Position coordinates
	 */
	position: {
		x: number;
		y: number;
	};

	/**
	 * Delay before showing (ms)
	 */
	openDelay: number;

	/**
	 * Delay before hiding (ms)
	 */
	closeDelay: number;

	/**
	 * Whether tooltip is disabled
	 */
	disabled: boolean;
}

/**
 * Tooltip configuration
 */
export interface TooltipConfig {
	/**
	 * Custom tooltip ID
	 */
	id?: string;

	/**
	 * Initial open state
	 */
	initialOpen?: boolean;

	/**
	 * Preferred placement
	 */
	placement?: TooltipPlacement;

	/**
	 * Delay before showing (ms)
	 */
	openDelay?: number;

	/**
	 * Delay before hiding (ms)
	 */
	closeDelay?: number;

	/**
	 * Offset from trigger (px)
	 */
	offset?: number;

	/**
	 * Enable smart positioning (avoid viewport edges)
	 */
	smartPositioning?: boolean;

	/**
	 * Show on hover
	 */
	showOnHover?: boolean;

	/**
	 * Show on focus
	 */
	showOnFocus?: boolean;

	/**
	 * Whether tooltip is disabled
	 */
	disabled?: boolean;

	/**
	 * Called when open state changes
	 */
	onOpenChange?: (open: boolean) => void;

	/**
	 * Called when tooltip opens
	 */
	onOpen?: () => void;

	/**
	 * Called when tooltip closes
	 */
	onClose?: () => void;

	/**
	 * Called on destroy
	 */
	onDestroy?: () => void;
}

/**
 * Tooltip actions
 */
export interface TooltipActions {
	/**
	 * Action for trigger element
	 */
	trigger: Action<HTMLElement>;

	/**
	 * Action for tooltip content
	 */
	content: Action<HTMLElement>;
}

/**
 * Tooltip helpers
 */
export interface TooltipHelpers {
	/**
	 * Show the tooltip
	 */
	show: () => void;

	/**
	 * Hide the tooltip
	 */
	hide: () => void;

	/**
	 * Open the tooltip (alias for show)
	 */
	open: () => void;

	/**
	 * Close the tooltip (alias for hide)
	 */
	close: () => void;

	/**
	 * Update tooltip position
	 */
	updatePosition: () => void;

	/**
	 * Set placement
	 */
	setPlacement: (placement: TooltipPlacement) => void;

	/**
	 * Set disabled state
	 */
	setDisabled: (disabled: boolean) => void;

	/**
	 * Set open delay
	 */
	setOpenDelay: (delay: number) => void;

	/**
	 * Set close delay
	 */
	setCloseDelay: (delay: number) => void;
}

/**
 * Calculate tooltip position
 */
function calculatePosition(
	triggerRect: DOMRect,
	tooltipRect: DOMRect,
	placement: TooltipPlacement,
	offset: number,
	smartPositioning: boolean
): { x: number; y: number; finalPlacement: TooltipPlacement } {
	let x = 0;
	let y = 0;
	let finalPlacement = placement;

	// Calculate initial position based on placement
	switch (placement) {
		case 'top':
			x = triggerRect.left + triggerRect.width / 2 - tooltipRect.width / 2;
			y = triggerRect.top - tooltipRect.height - offset;
			break;
		case 'top-start':
			x = triggerRect.left;
			y = triggerRect.top - tooltipRect.height - offset;
			break;
		case 'top-end':
			x = triggerRect.right - tooltipRect.width;
			y = triggerRect.top - tooltipRect.height - offset;
			break;
		case 'bottom':
			x = triggerRect.left + triggerRect.width / 2 - tooltipRect.width / 2;
			y = triggerRect.bottom + offset;
			break;
		case 'bottom-start':
			x = triggerRect.left;
			y = triggerRect.bottom + offset;
			break;
		case 'bottom-end':
			x = triggerRect.right - tooltipRect.width;
			y = triggerRect.bottom + offset;
			break;
		case 'left':
			x = triggerRect.left - tooltipRect.width - offset;
			y = triggerRect.top + triggerRect.height / 2 - tooltipRect.height / 2;
			break;
		case 'left-start':
			x = triggerRect.left - tooltipRect.width - offset;
			y = triggerRect.top;
			break;
		case 'left-end':
			x = triggerRect.left - tooltipRect.width - offset;
			y = triggerRect.bottom - tooltipRect.height;
			break;
		case 'right':
			x = triggerRect.right + offset;
			y = triggerRect.top + triggerRect.height / 2 - tooltipRect.height / 2;
			break;
		case 'right-start':
			x = triggerRect.right + offset;
			y = triggerRect.top;
			break;
		case 'right-end':
			x = triggerRect.right + offset;
			y = triggerRect.bottom - tooltipRect.height;
			break;
	}

	// Smart positioning: flip if near viewport edges
	if (smartPositioning) {
		const viewportWidth = window.innerWidth;
		const viewportHeight = window.innerHeight;
		const margin = 8; // Minimum distance from viewport edge

		// Check if tooltip goes off right edge
		if (x + tooltipRect.width > viewportWidth - margin) {
			if (placement.startsWith('right')) {
				// Flip to left
				finalPlacement = placement.replace('right', 'left') as TooltipPlacement;
				x = triggerRect.left - tooltipRect.width - offset;
			} else {
				// Adjust to fit
				x = viewportWidth - tooltipRect.width - margin;
			}
		}

		// Check if tooltip goes off left edge
		if (x < margin) {
			if (placement.startsWith('left')) {
				// Flip to right
				finalPlacement = placement.replace('left', 'right') as TooltipPlacement;
				x = triggerRect.right + offset;
			} else {
				// Adjust to fit
				x = margin;
			}
		}

		// Check if tooltip goes off bottom edge
		if (y + tooltipRect.height > viewportHeight - margin) {
			if (placement.startsWith('bottom')) {
				// Flip to top
				finalPlacement = placement.replace('bottom', 'top') as TooltipPlacement;
				y = triggerRect.top - tooltipRect.height - offset;
			} else {
				// Adjust to fit
				y = viewportHeight - tooltipRect.height - margin;
			}
		}

		// Check if tooltip goes off top edge
		if (y < margin) {
			if (placement.startsWith('top')) {
				// Flip to bottom
				finalPlacement = placement.replace('top', 'bottom') as TooltipPlacement;
				y = triggerRect.bottom + offset;
			} else {
				// Adjust to fit
				y = margin;
			}
		}
	}

	return { x, y, finalPlacement };
}

/**
 * Create a headless tooltip primitive
 */
export function createTooltip(config: TooltipConfig = {}) {
	const {
		id: customId,
		initialOpen = false,
		placement: preferredPlacement = 'top',
		openDelay = 300,
		closeDelay = 0,
		offset = 8,
		smartPositioning = true,
		showOnHover = true,
		showOnFocus = true,
		disabled = false,
		onOpen,
		onClose,
		onOpenChange,
		onDestroy,
	} = config;

	// Reactive state
	const internalState: TooltipState = {
		open: initialOpen,
		id: customId || generateId('tooltip'),
		placement: preferredPlacement,
		position: { x: 0, y: 0 },
		openDelay,
		closeDelay,
		disabled,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof TooltipState, value) {
			const oldValue = target[prop];
			target[prop] = value as never;
			if (prop === 'open' && oldValue !== value) {
				onOpenChange?.(value as boolean);
				if (value) {
					onOpen?.();
				} else {
					onClose?.();
				}
			}
			return true;
		},
	});

	let triggerElement: HTMLElement | null = null;
	let contentElement: HTMLElement | null = null;
	let showTimeoutId: number | undefined = undefined;
	let hideTimeoutId: number | undefined = undefined;

	/**
	 * Update tooltip position
	 */
	function updatePosition() {
		if (!triggerElement || !contentElement) return;

		const triggerRect = triggerElement.getBoundingClientRect();
		const tooltipRect = contentElement.getBoundingClientRect();

		const { x, y, finalPlacement } = calculatePosition(
			triggerRect,
			tooltipRect,
			preferredPlacement,
			offset,
			smartPositioning
		);

		state.position = { x, y };
		state.placement = finalPlacement;

		// Apply position to content element
		contentElement.style.position = 'fixed';
		contentElement.style.left = `${x}px`;
		contentElement.style.top = `${y}px`;
	}

	/**
	 * Show the tooltip (with delay)
	 */
	function show() {
		if (state.open || state.disabled) return;

		// Clear hide timeout
		if (hideTimeoutId !== undefined) {
			window.clearTimeout(hideTimeoutId);
			hideTimeoutId = undefined;
		}

		// If delay is 0, show immediately (synchronously)
		if (state.openDelay === 0) {
			state.open = true;
			setTimeout(updatePosition, 0);
			return;
		}

		// Set show timeout
		showTimeoutId = window.setTimeout(() => {
			state.open = true;
			showTimeoutId = undefined;

			// Update position after showing
			setTimeout(updatePosition, 0);
		}, state.openDelay);
	}

	/**
	 * Hide the tooltip (with delay)
	 */
	function hide() {
		if (!state.open && showTimeoutId === undefined) return;

		// Clear show timeout
		if (showTimeoutId !== undefined) {
			window.clearTimeout(showTimeoutId);
			showTimeoutId = undefined;
			return;
		}

		// If delay is 0, hide immediately (synchronously)
		if (state.closeDelay === 0) {
			state.open = false;
			return;
		}

		// Set hide timeout
		hideTimeoutId = window.setTimeout(() => {
			state.open = false;
			hideTimeoutId = undefined;
		}, state.closeDelay);
	}

	/**
	 * Trigger action
	 */
	const trigger: Action<HTMLElement> = (node: HTMLElement) => {
		triggerElement = node;

		function handleMouseEnter() {
			if (showOnHover) show();
		}

		function handleMouseLeave() {
			if (showOnHover) hide();
		}

		function handleFocus() {
			if (showOnFocus) show();
		}

		function handleBlur() {
			if (showOnFocus) hide();
		}

		if (showOnHover) {
			node.addEventListener('mouseenter', handleMouseEnter);
			node.addEventListener('mouseleave', handleMouseLeave);
		}

		if (showOnFocus) {
			node.addEventListener('focus', handleFocus);
			node.addEventListener('blur', handleBlur);
		}

		node.setAttribute('aria-describedby', state.id);

		return {
			destroy() {
				if (showOnHover) {
					node.removeEventListener('mouseenter', handleMouseEnter);
					node.removeEventListener('mouseleave', handleMouseLeave);
				}

				if (showOnFocus) {
					node.removeEventListener('focus', handleFocus);
					node.removeEventListener('blur', handleBlur);
				}

				// Clear any pending timeouts
				if (showTimeoutId !== undefined) {
					window.clearTimeout(showTimeoutId);
				}
				if (hideTimeoutId !== undefined) {
					window.clearTimeout(hideTimeoutId);
				}

				triggerElement = null;
				onDestroy?.();
			},
		};
	};

	/**
	 * Content action
	 */
	const content: Action<HTMLElement> = (node: HTMLElement) => {
		contentElement = node;

		node.setAttribute('role', 'tooltip');
		node.setAttribute('id', state.id);
		node.setAttribute('data-placement', state.placement);

		// Update position on scroll/resize
		const handleUpdate = () => {
			if (state.open) updatePosition();
		};

		// Keep tooltip open when hovering over it
		function handleMouseEnter() {
			if (hideTimeoutId !== undefined) {
				window.clearTimeout(hideTimeoutId);
				hideTimeoutId = undefined;
			}
		}

		function handleMouseLeave() {
			hide();
		}

		node.addEventListener('mouseenter', handleMouseEnter);
		node.addEventListener('mouseleave', handleMouseLeave);
		window.addEventListener('scroll', handleUpdate, true);
		window.addEventListener('resize', handleUpdate);

		return {
			update() {
				node.setAttribute('data-placement', state.placement);
			},
			destroy() {
				node.removeEventListener('mouseenter', handleMouseEnter);
				node.removeEventListener('mouseleave', handleMouseLeave);
				window.removeEventListener('scroll', handleUpdate, true);
				window.removeEventListener('resize', handleUpdate);
				contentElement = null;
			},
		};
	};

	const actions: TooltipActions = {
		trigger,
		content,
	};

	/**
	 * Open tooltip immediately (no delay)
	 */
	function open() {
		if (state.disabled) return;

		// Clear any pending timeouts
		if (showTimeoutId !== undefined) {
			window.clearTimeout(showTimeoutId);
			showTimeoutId = undefined;
		}
		if (hideTimeoutId !== undefined) {
			window.clearTimeout(hideTimeoutId);
			hideTimeoutId = undefined;
		}

		state.open = true;

		// Update position after showing
		setTimeout(updatePosition, 0);
	}

	/**
	 * Close tooltip immediately (no delay)
	 */
	function close() {
		// Clear any pending timeouts
		if (showTimeoutId !== undefined) {
			window.clearTimeout(showTimeoutId);
			showTimeoutId = undefined;
		}
		if (hideTimeoutId !== undefined) {
			window.clearTimeout(hideTimeoutId);
			hideTimeoutId = undefined;
		}

		state.open = false;
	}

	/**
	 * Set placement
	 */
	function setPlacement(placement: TooltipPlacement) {
		state.placement = placement;
		if (contentElement) {
			contentElement.setAttribute('data-placement', placement);
		}
	}

	/**
	 * Set disabled state
	 */
	function setDisabled(disabled: boolean) {
		state.disabled = disabled;
		if (disabled && state.open) {
			state.open = false;
		}
	}

	/**
	 * Set open delay
	 */
	function setOpenDelay(delay: number) {
		state.openDelay = delay;
	}

	/**
	 * Set close delay
	 */
	function setCloseDelay(delay: number) {
		state.closeDelay = delay;
	}

	const helpers: TooltipHelpers = {
		show,
		hide,
		open,
		close,
		updatePosition,
		setPlacement,
		setDisabled,
		setOpenDelay,
		setCloseDelay,
	};

	return {
		state,
		actions,
		helpers,
	};
}
