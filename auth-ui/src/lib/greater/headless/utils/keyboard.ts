/**
 * Keyboard utilities for headless components
 * @module @equaltoai/greater-components-headless/utils/keyboard
 */

/**
 * Check if a keyboard event represents an activation key (Enter or Space)
 */
export function isActivationKey(event: KeyboardEvent): boolean {
	return event.key === 'Enter' || event.key === ' ' || event.key === 'Spacebar';
}

/**
 * Check if a keyboard event represents the Escape key
 */
export function isEscapeKey(event: KeyboardEvent): boolean {
	return event.key === 'Escape' || event.key === 'Esc';
}

/**
 * Check if a keyboard event represents an arrow key
 */
export function isArrowKey(event: KeyboardEvent): boolean {
	return ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(event.key);
}

/**
 * Check if a keyboard event represents a Tab key
 */
export function isTabKey(event: KeyboardEvent): boolean {
	return event.key === 'Tab';
}

/**
 * Get the direction from an arrow key press
 * @returns 'up' | 'down' | 'left' | 'right' | null
 */
export function getArrowKeyDirection(
	event: KeyboardEvent
): 'up' | 'down' | 'left' | 'right' | null {
	switch (event.key) {
		case 'ArrowUp':
			return 'up';
		case 'ArrowDown':
			return 'down';
		case 'ArrowLeft':
			return 'left';
		case 'ArrowRight':
			return 'right';
		default:
			return null;
	}
}

/**
 * Keyboard navigation direction
 */
export type NavigationDirection = 'next' | 'previous' | 'first' | 'last';

/**
 * Get navigation direction from keyboard event
 */
export function getNavigationDirection(
	event: KeyboardEvent,
	orientation: 'horizontal' | 'vertical' = 'vertical'
): NavigationDirection | null {
	if (event.key === 'Home') return 'first';
	if (event.key === 'End') return 'last';

	if (orientation === 'vertical') {
		if (event.key === 'ArrowDown') return 'next';
		if (event.key === 'ArrowUp') return 'previous';
	} else {
		if (event.key === 'ArrowRight') return 'next';
		if (event.key === 'ArrowLeft') return 'previous';
	}

	return null;
}
