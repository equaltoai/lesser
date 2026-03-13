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
export function calculatePlacement(config: PositionConfig): MenuPlacement {
	const { triggerRect, contentRect, placement, offset, viewportMargin = 8 } = config;

	const viewportWidth = window.innerWidth;
	const viewportHeight = window.innerHeight;

	const { x, y } = (() => {
		switch (placement) {
			case 'bottom-start':
				return { x: triggerRect.left, y: triggerRect.bottom + offset };
			case 'bottom-end':
				return { x: triggerRect.right - contentRect.width, y: triggerRect.bottom + offset };
			case 'top-start':
				return { x: triggerRect.left, y: triggerRect.top - contentRect.height - offset };
			case 'top-end':
				return {
					x: triggerRect.right - contentRect.width,
					y: triggerRect.top - contentRect.height - offset,
				};
		}
	})();

	let finalPlacement = placement;

	// Viewport boundary detection and auto-flip
	const wouldOverflowBottom = y + contentRect.height > viewportHeight - viewportMargin;
	const wouldOverflowTop = y < viewportMargin;

	// Vertical flip
	if (placement.startsWith('bottom') && wouldOverflowBottom) {
		const topY = triggerRect.top - contentRect.height - offset;
		if (topY >= viewportMargin) {
			finalPlacement = placement.replace('bottom', 'top') as MenuPlacement;
		}
	} else if (placement.startsWith('top') && wouldOverflowTop) {
		const bottomY = triggerRect.bottom + offset;
		if (bottomY + contentRect.height <= viewportHeight - viewportMargin) {
			finalPlacement = placement.replace('top', 'bottom') as MenuPlacement;
		}
	}

	// Horizontal adjustment
	const wouldOverflowRight = x + contentRect.width > viewportWidth - viewportMargin;
	const wouldOverflowLeft = x < viewportMargin;
	if (wouldOverflowRight) {
		if (finalPlacement.endsWith('-start')) {
			finalPlacement = finalPlacement.replace('-start', '-end') as MenuPlacement;
		}
	} else if (wouldOverflowLeft) {
		if (finalPlacement.endsWith('-end')) {
			finalPlacement = finalPlacement.replace('-end', '-start') as MenuPlacement;
		}
	}

	return finalPlacement;
}

/**
 * Get scroll parent of an element
 */
export function getScrollParent(element: HTMLElement): HTMLElement | null {
	let parent = element.parentElement;

	while (parent) {
		const style = getComputedStyle(parent);
		const overflow = style.overflow + style.overflowY + style.overflowX;

		if (/(auto|scroll|overlay)/.test(overflow)) {
			return parent;
		}

		parent = parent.parentElement;
	}

	return null;
}

/**
 * Create a resize/scroll observer for repositioning
 */
export function createPositionObserver(
	triggerElement: HTMLElement,
	onUpdate: () => void
): () => void {
	const scrollParent = getScrollParent(triggerElement);

	// Observe resize
	const resizeObserver = new ResizeObserver(onUpdate);
	resizeObserver.observe(document.body);

	// Listen for scroll events
	const scrollHandler = () => requestAnimationFrame(onUpdate);
	window.addEventListener('scroll', scrollHandler, { passive: true });
	scrollParent?.addEventListener('scroll', scrollHandler, { passive: true });
	window.addEventListener('resize', scrollHandler, { passive: true });

	// Cleanup function
	return () => {
		resizeObserver.disconnect();
		window.removeEventListener('scroll', scrollHandler);
		scrollParent?.removeEventListener('scroll', scrollHandler);
		window.removeEventListener('resize', scrollHandler);
	};
}
