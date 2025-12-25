/**
 * ID generation utilities for headless components
 * @module @equaltoai/greater-components-headless/utils/id
 */

let idCounter = 0;

/**
 * Generate a unique ID for component elements
 * @param prefix - Optional prefix for the ID
 * @returns Unique ID string
 */
export function generateId(prefix = 'greater'): string {
	return `${prefix}-${++idCounter}`;
}

/**
 * Ensure an element has an ID, generating one if needed
 * @param element - The HTML element
 * @param prefix - Optional prefix for generated ID
 * @returns The element's ID
 */
export function ensureId(element: HTMLElement, prefix = 'greater'): string {
	if (!element.id) {
		element.id = generateId(prefix);
	}
	return element.id;
}

/**
 * Reset the ID counter (useful for testing)
 */
export function resetIdCounter(): void {
	idCounter = 0;
}
