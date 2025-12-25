/**
 * Popover Positioning Behavior
 *
 * Framework-agnostic utility for smart positioning of floating elements.
 * Handles tooltips, dropdowns, and popovers with viewport-aware placement.
 *
 * @module @equaltoai/greater-components-headless/behaviors/popover
 */

/**
 * Placement options
 */
export type PopoverPlacement =
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
 * Alignment options
 */
export type PopoverAlignment = 'start' | 'center' | 'end';

/**
 * Side options
 */
export type PopoverSide = 'top' | 'bottom' | 'left' | 'right';

/**
 * Popover configuration
 */
export interface PopoverConfig {
	/**
	 * Preferred placement
	 * @default 'bottom'
	 */
	placement?: PopoverPlacement;

	/**
	 * Offset from reference element in pixels
	 * @default 8
	 */
	offset?: number;

	/**
	 * Minimum distance from viewport edge
	 * @default 8
	 */
	viewportPadding?: number;

	/**
	 * Whether to flip placement when overflowing
	 * @default true
	 */
	flip?: boolean;

	/**
	 * Whether to shift along axis to stay in viewport
	 * @default true
	 */
	shift?: boolean;

	/**
	 * Boundary element for positioning
	 * @default viewport
	 */
	boundary?: HTMLElement | 'viewport';

	/**
	 * Strategy for positioning
	 * @default 'absolute'
	 */
	strategy?: 'absolute' | 'fixed';

	/**
	 * Callback when position is computed
	 */
	onPositionChange?: (position: PopoverPosition) => void;
}

/**
 * Computed position
 */
export interface PopoverPosition {
	/**
	 * X coordinate
	 */
	x: number;

	/**
	 * Y coordinate
	 */
	y: number;

	/**
	 * Final placement after flip/shift
	 */
	placement: PopoverPlacement;

	/**
	 * Arrow position (if applicable)
	 */
	arrow?: {
		x: number;
		y: number;
	};

	/**
	 * Whether placement was flipped
	 */
	flipped: boolean;

	/**
	 * Whether position was shifted
	 */
	shifted: boolean;
}

/**
 * Popover state
 */
export interface PopoverState {
	/**
	 * Reference element
	 */
	reference: HTMLElement | null;

	/**
	 * Floating element
	 */
	floating: HTMLElement | null;

	/**
	 * Current position
	 */
	position: PopoverPosition | null;

	/**
	 * Whether popover is visible
	 */
	visible: boolean;
}

/**
 * Popover return type
 */
export interface Popover {
	/**
	 * Current state
	 */
	state: PopoverState;

	/**
	 * Set reference element
	 */
	setReference: (element: HTMLElement | null) => void;

	/**
	 * Set floating element
	 */
	setFloating: (element: HTMLElement | null) => void;

	/**
	 * Compute and update position
	 */
	update: () => PopoverPosition | null;

	/**
	 * Show the popover
	 */
	show: () => void;

	/**
	 * Hide the popover
	 */
	hide: () => void;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<PopoverConfig>) => void;

	/**
	 * Destroy and clean up
	 */
	destroy: () => void;
}

/**
 * Parse placement into side and alignment
 */
function parsePlacement(placement: PopoverPlacement): {
	side: PopoverSide;
	alignment: PopoverAlignment;
} {
	const parts = placement.split('-');
	const side = parts[0] as PopoverSide;
	const alignment = (parts[1] as PopoverAlignment) || 'center';
	return { side, alignment };
}

/**
 * Get opposite side
 */
function getOppositeSide(side: PopoverSide): PopoverSide {
	const opposites: Record<PopoverSide, PopoverSide> = {
		top: 'bottom',
		bottom: 'top',
		left: 'right',
		right: 'left',
	};
	return opposites[side];
}

/**
 * Construct placement from side and alignment
 */
function constructPlacement(side: PopoverSide, alignment: PopoverAlignment): PopoverPlacement {
	if (alignment === 'center') {
		return side;
	}
	return `${side}-${alignment}` as PopoverPlacement;
}

/**
 * Get viewport rect
 */
function getViewportRect(): DOMRect {
	return new DOMRect(
		0,
		0,
		document.documentElement.clientWidth,
		document.documentElement.clientHeight
	);
}

/**
 * Get boundary rect
 */
function getBoundaryRect(boundary: HTMLElement | 'viewport'): DOMRect {
	if (boundary === 'viewport') {
		return getViewportRect();
	}
	return boundary.getBoundingClientRect();
}

/**
 * Create a popover positioning manager
 */
export function createPopover(config: PopoverConfig = {}): Popover {
	let currentConfig: Required<PopoverConfig> = {
		placement: config.placement ?? 'bottom',
		offset: config.offset ?? 8,
		viewportPadding: config.viewportPadding ?? 8,
		flip: config.flip ?? true,
		shift: config.shift ?? true,
		boundary: config.boundary ?? 'viewport',
		strategy: config.strategy ?? 'absolute',
		onPositionChange: config.onPositionChange ?? (() => {}),
	};

	const state: PopoverState = {
		reference: null,
		floating: null,
		position: null,
		visible: false,
	};

	/**
	 * Compute position for a given placement
	 */
	function computePositionForPlacement(
		referenceRect: DOMRect,
		floatingRect: DOMRect,
		placement: PopoverPlacement
	): { x: number; y: number } {
		const { side, alignment } = parsePlacement(placement);
		const { offset } = currentConfig;

		let x = 0;
		let y = 0;

		// Calculate base position based on side
		switch (side) {
			case 'top':
				x = referenceRect.left + referenceRect.width / 2 - floatingRect.width / 2;
				y = referenceRect.top - floatingRect.height - offset;
				break;
			case 'bottom':
				x = referenceRect.left + referenceRect.width / 2 - floatingRect.width / 2;
				y = referenceRect.bottom + offset;
				break;
			case 'left':
				x = referenceRect.left - floatingRect.width - offset;
				y = referenceRect.top + referenceRect.height / 2 - floatingRect.height / 2;
				break;
			case 'right':
				x = referenceRect.right + offset;
				y = referenceRect.top + referenceRect.height / 2 - floatingRect.height / 2;
				break;
		}

		// Adjust for alignment
		if (side === 'top' || side === 'bottom') {
			if (alignment === 'start') {
				x = referenceRect.left;
			} else if (alignment === 'end') {
				x = referenceRect.right - floatingRect.width;
			}
		} else {
			if (alignment === 'start') {
				y = referenceRect.top;
			} else if (alignment === 'end') {
				y = referenceRect.bottom - floatingRect.height;
			}
		}

		return { x, y };
	}

	/**
	 * Check if position overflows boundary
	 */
	function checkOverflow(
		x: number,
		y: number,
		floatingRect: DOMRect,
		boundaryRect: DOMRect
	): { top: number; right: number; bottom: number; left: number } {
		const { viewportPadding } = currentConfig;

		return {
			top: boundaryRect.top + viewportPadding - y,
			right: x + floatingRect.width - (boundaryRect.right - viewportPadding),
			bottom: y + floatingRect.height - (boundaryRect.bottom - viewportPadding),
			left: boundaryRect.left + viewportPadding - x,
		};
	}

	/**
	 * Compute optimal position
	 */
	function computePosition(): PopoverPosition | null {
		if (!state.reference || !state.floating) return null;

		const referenceRect = state.reference.getBoundingClientRect();
		const floatingRect = state.floating.getBoundingClientRect();
		const boundaryRect = getBoundaryRect(currentConfig.boundary);

		let { placement } = currentConfig;
		let { x, y } = computePositionForPlacement(referenceRect, floatingRect, placement);
		let flipped = false;
		let shifted = false;

		// Check overflow
		let overflow = checkOverflow(x, y, floatingRect, boundaryRect);

		// Flip if needed
		if (currentConfig.flip) {
			const { side, alignment } = parsePlacement(placement);

			// Check if we need to flip
			const shouldFlip =
				(side === 'top' && overflow.top > 0) ||
				(side === 'bottom' && overflow.bottom > 0) ||
				(side === 'left' && overflow.left > 0) ||
				(side === 'right' && overflow.right > 0);

			if (shouldFlip) {
				const oppositeSide = getOppositeSide(side);
				const flippedPlacement = constructPlacement(oppositeSide, alignment);
				const flippedPos = computePositionForPlacement(
					referenceRect,
					floatingRect,
					flippedPlacement
				);
				const flippedOverflow = checkOverflow(
					flippedPos.x,
					flippedPos.y,
					floatingRect,
					boundaryRect
				);

				// Use flipped position if it has less overflow
				const currentOverflowSum =
					Math.max(0, overflow.top) +
					Math.max(0, overflow.bottom) +
					Math.max(0, overflow.left) +
					Math.max(0, overflow.right);
				const flippedOverflowSum =
					Math.max(0, flippedOverflow.top) +
					Math.max(0, flippedOverflow.bottom) +
					Math.max(0, flippedOverflow.left) +
					Math.max(0, flippedOverflow.right);

				if (flippedOverflowSum < currentOverflowSum) {
					placement = flippedPlacement;
					x = flippedPos.x;
					y = flippedPos.y;
					overflow = flippedOverflow;
					flipped = true;
				}
			}
		}

		// Shift if needed
		if (currentConfig.shift) {
			const { side } = parsePlacement(placement);

			if (side === 'top' || side === 'bottom') {
				// Shift horizontally
				if (overflow.left > 0) {
					x += overflow.left;
					shifted = true;
				} else if (overflow.right > 0) {
					x -= overflow.right;
					shifted = true;
				}
			} else {
				// Shift vertically
				if (overflow.top > 0) {
					y += overflow.top;
					shifted = true;
				} else if (overflow.bottom > 0) {
					y -= overflow.bottom;
					shifted = true;
				}
			}
		}

		// Calculate arrow position
		const { side } = parsePlacement(placement);
		let arrow: { x: number; y: number } | undefined;

		if (side === 'top' || side === 'bottom') {
			arrow = {
				x: referenceRect.left + referenceRect.width / 2 - x,
				y: side === 'top' ? floatingRect.height : 0,
			};
		} else {
			arrow = {
				x: side === 'left' ? floatingRect.width : 0,
				y: referenceRect.top + referenceRect.height / 2 - y,
			};
		}

		const position: PopoverPosition = {
			x,
			y,
			placement,
			arrow,
			flipped,
			shifted,
		};

		state.position = position;
		currentConfig.onPositionChange(position);

		return position;
	}

	/**
	 * Update position and apply to floating element
	 */
	function update(): PopoverPosition | null {
		const position = computePosition();

		if (position && state.floating) {
			state.floating.style.position = currentConfig.strategy;
			state.floating.style.left = `${position.x}px`;
			state.floating.style.top = `${position.y}px`;
		}

		return position;
	}

	/**
	 * Set reference element
	 */
	function setReference(element: HTMLElement | null): void {
		state.reference = element;
		if (state.visible) {
			update();
		}
	}

	/**
	 * Set floating element
	 */
	function setFloating(element: HTMLElement | null): void {
		state.floating = element;
		if (state.visible) {
			update();
		}
	}

	/**
	 * Show the popover
	 */
	function show(): void {
		state.visible = true;
		update();
	}

	/**
	 * Hide the popover
	 */
	function hide(): void {
		state.visible = false;
		state.position = null;
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<PopoverConfig>): void {
		currentConfig = {
			...currentConfig,
			...newConfig,
			onPositionChange: newConfig.onPositionChange ?? currentConfig.onPositionChange,
		};

		if (state.visible) {
			update();
		}
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		state.reference = null;
		state.floating = null;
		state.position = null;
		state.visible = false;
	}

	return {
		state,
		setReference,
		setFloating,
		update,
		show,
		hide,
		updateConfig,
		destroy,
	};
}
