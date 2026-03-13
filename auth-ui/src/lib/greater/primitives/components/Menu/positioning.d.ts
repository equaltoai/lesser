/**
 * Menu Positioning Utilities
 * Handles dropdown positioning with viewport boundary detection and auto-flip
 * @module @equaltoai/greater-components/primitives/Menu/positioning
 */
import type { MenuPlacement } from './context.svelte';
export interface PositionConfig {
	triggerRect: DOMRect;
	contentRect: DOMRect;
	placement: MenuPlacement;
	offset: number;
	viewportMargin?: number;
}
/**
 * Calculate the best placement based on trigger element and preferred placement.
 * Returns a placement string only (no pixel positioning) for strict CSP compatibility.
 */
export declare function calculatePlacement(config: PositionConfig): MenuPlacement;
/**
 * Get scroll parent of an element
 */
export declare function getScrollParent(element: HTMLElement): HTMLElement | null;
/**
 * Create a resize/scroll observer for repositioning
 */
export declare function createPositionObserver(
	triggerElement: HTMLElement,
	onUpdate: () => void
): () => void;
//# sourceMappingURL=positioning.d.ts.map
