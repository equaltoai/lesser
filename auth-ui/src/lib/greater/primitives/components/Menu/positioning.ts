/**
 * Menu Positioning Utilities
 * Handles dropdown positioning with viewport boundary detection and auto-flip
 * @module @equaltoai/greater-components/primitives/Menu/positioning
 */

import type { MenuPlacement, MenuPosition } from './context.svelte';

export interface PositionConfig {
	triggerRect: DOMRect;
	contentRect: DOMRect;
	placement: MenuPlacement;
	offset: number;
	viewportMargin?: number;
}

/**
 * Calculate menu position based on trigger element and preferred placement
 */
export function calculatePosition(config: PositionConfig): MenuPosition {
	const { triggerRect, contentRect, placement, offset, viewportMargin = 8 } = config;

	const viewportWidth = window.innerWidth;
	const viewportHeight = window.innerHeight;

	let x = 0;
	let y = 0;
	let finalPlacement = placement;

	// Calculate initial position based on placement
	switch (placement) {
		case 'bottom-start':
			x = triggerRect.left;
			y = triggerRect.bottom + offset;
			break;
		case 'bottom-end':
			x = triggerRect.right - contentRect.width;
			y = triggerRect.bottom + offset;
			break;
		case 'top-start':
			x = triggerRect.left;
			y = triggerRect.top - contentRect.height - offset;
			break;
		case 'top-end':
			x = triggerRect.right - contentRect.width;
			y = triggerRect.top - contentRect.height - offset;
			break;
	}

	// Viewport boundary detection and auto-flip
	const wouldOverflowBottom = y + contentRect.height > viewportHeight - viewportMargin;
	const wouldOverflowTop = y < viewportMargin;
	const wouldOverflowRight = x + contentRect.width > viewportWidth - viewportMargin;
	const wouldOverflowLeft = x < viewportMargin;

	// Vertical flip
	if (placement.startsWith('bottom') && wouldOverflowBottom) {
		const topY = triggerRect.top - contentRect.height - offset;
		if (topY >= viewportMargin) {
			y = topY;
			finalPlacement = placement.replace('bottom', 'top') as MenuPlacement;
		}
	} else if (placement.startsWith('top') && wouldOverflowTop) {
		const bottomY = triggerRect.bottom + offset;
		if (bottomY + contentRect.height <= viewportHeight - viewportMargin) {
			y = bottomY;
			finalPlacement = placement.replace('top', 'bottom') as MenuPlacement;
		}
	}

	// Horizontal adjustment
	if (wouldOverflowRight) {
		x = Math.max(viewportMargin, viewportWidth - contentRect.width - viewportMargin);
		if (finalPlacement.endsWith('-start')) {
			finalPlacement = finalPlacement.replace('-start', '-end') as MenuPlacement;
		}
	} else if (wouldOverflowLeft) {
		x = viewportMargin;
		if (finalPlacement.endsWith('-end')) {
			finalPlacement = finalPlacement.replace('-end', '-start') as MenuPlacement;
		}
	}

	// Ensure within viewport bounds
	x = Math.max(viewportMargin, Math.min(x, viewportWidth - contentRect.width - viewportMargin));
	y = Math.max(viewportMargin, Math.min(y, viewportHeight - contentRect.height - viewportMargin));

	return { x, y, placement: finalPlacement };
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
