/**
 * Dismissable Behavior
 *
 * Framework-agnostic utility for click-outside and ESC key handling.
 * Supports nested dismissables through a layer stack.
 *
 * @module @equaltoai/greater-components-headless/behaviors/dismissable
 */

/**
 * Dismissable configuration
 */
export interface DismissableConfig {
	/**
	 * Whether to dismiss on click outside
	 * @default true
	 */
	closeOnClickOutside?: boolean;

	/**
	 * Whether to dismiss on ESC key
	 * @default true
	 */
	closeOnEscape?: boolean;

	/**
	 * Whether to dismiss on scroll outside
	 * @default false
	 */
	closeOnScroll?: boolean;

	/**
	 * Whether to dismiss on resize
	 * @default false
	 */
	closeOnResize?: boolean;

	/**
	 * Whether to dismiss on focus outside
	 * @default false
	 */
	closeOnFocusOutside?: boolean;

	/**
	 * Elements that should not trigger dismiss when clicked
	 */
	excludeElements?: HTMLElement[];

	/**
	 * Callback when dismiss is triggered
	 */
	onDismiss?: (reason: DismissReason) => void;

	/**
	 * Callback before dismiss (can prevent dismiss by returning false)
	 */
	onBeforeDismiss?: (reason: DismissReason) => boolean;
}

/**
 * Reason for dismissal
 */
export type DismissReason =
	| 'click-outside'
	| 'escape'
	| 'scroll'
	| 'resize'
	| 'focus-outside'
	| 'programmatic';

/**
 * Dismissable state
 */
export interface DismissableState {
	/**
	 * Target element
	 */
	target: HTMLElement | null;

	/**
	 * Whether dismissable is active
	 */
	active: boolean;

	/**
	 * Layer index in stack
	 */
	layerIndex: number;
}

/**
 * Dismissable return type
 */
export interface Dismissable {
	/**
	 * Current state
	 */
	state: DismissableState;

	/**
	 * Activate dismissable on target element
	 */
	activate: (target: HTMLElement) => void;

	/**
	 * Deactivate dismissable
	 */
	deactivate: () => void;

	/**
	 * Programmatically trigger dismiss
	 */
	dismiss: () => void;

	/**
	 * Add element to exclude list
	 */
	addExclude: (element: HTMLElement) => void;

	/**
	 * Remove element from exclude list
	 */
	removeExclude: (element: HTMLElement) => void;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<DismissableConfig>) => void;

	/**
	 * Destroy and clean up
	 */
	destroy: () => void;
}

/**
 * Global layer stack for nested dismissables
 */
const layerStack: Dismissable[] = [];

/**
 * Check if this dismissable is the top layer
 */
function isTopLayer(dismissable: Dismissable): boolean {
	return layerStack[layerStack.length - 1] === dismissable;
}

/**
 * Create a dismissable manager
 */
export function createDismissable(config: DismissableConfig = {}): Dismissable {
	let currentConfig: Required<DismissableConfig> = {
		closeOnClickOutside: config.closeOnClickOutside ?? true,
		closeOnEscape: config.closeOnEscape ?? true,
		closeOnScroll: config.closeOnScroll ?? false,
		closeOnResize: config.closeOnResize ?? false,
		closeOnFocusOutside: config.closeOnFocusOutside ?? false,
		excludeElements: config.excludeElements ?? [],
		onDismiss: config.onDismiss ?? (() => {}),
		onBeforeDismiss: config.onBeforeDismiss ?? (() => true),
	};

	const state: DismissableState = {
		target: null,
		active: false,
		layerIndex: -1,
	};

	let clickHandler: ((event: MouseEvent) => void) | null = null;
	let keydownHandler: ((event: KeyboardEvent) => void) | null = null;
	let scrollHandler: ((event: Event) => void) | null = null;
	let resizeHandler: (() => void) | null = null;
	let focusinHandler: ((event: FocusEvent) => void) | null = null;

	// Reference to self for layer stack

	/**
	 * Check if element is excluded
	 */
	function isExcluded(element: HTMLElement): boolean {
		return currentConfig.excludeElements.some(
			(excluded) => excluded === element || excluded.contains(element)
		);
	}

	/**
	 * Check if click is outside target
	 */
	function isOutside(target: Node): boolean {
		if (!state.target) return false;
		return !state.target.contains(target) && !isExcluded(target as HTMLElement);
	}

	/**
	 * Trigger dismiss
	 */
	function triggerDismiss(reason: DismissReason): void {
		if (!state.active) return;

		// Only handle if this is the top layer
		if (!isTopLayer(selfRef)) return;

		// Check if dismiss should be prevented
		if (!currentConfig.onBeforeDismiss(reason)) return;

		currentConfig.onDismiss(reason);
	}

	/**
	 * Handle click outside
	 */
	function handleClick(event: MouseEvent): void {
		if (!currentConfig.closeOnClickOutside) return;
		if (!state.target) return;

		const target = event.target as Node;

		// Use setTimeout to handle click events that might open nested layers
		setTimeout(() => {
			if (isOutside(target)) {
				triggerDismiss('click-outside');
			}
		}, 0);
	}

	/**
	 * Handle ESC key
	 */
	function handleKeydown(event: KeyboardEvent): void {
		if (!currentConfig.closeOnEscape) return;
		if (event.key !== 'Escape') return;

		// Only handle if this is the top layer
		if (!isTopLayer(selfRef)) return;

		event.preventDefault();
		event.stopPropagation();
		triggerDismiss('escape');
	}

	/**
	 * Handle scroll outside
	 */
	function handleScroll(event: Event): void {
		if (!currentConfig.closeOnScroll) return;
		if (!state.target) return;

		const target = event.target as Node;
		if (isOutside(target)) {
			triggerDismiss('scroll');
		}
	}

	/**
	 * Handle resize
	 */
	function handleResize(): void {
		if (!currentConfig.closeOnResize) return;
		triggerDismiss('resize');
	}

	/**
	 * Handle focus outside
	 */
	function handleFocusin(event: FocusEvent): void {
		if (!currentConfig.closeOnFocusOutside) return;
		if (!state.target) return;

		const target = event.target as Node;
		if (isOutside(target)) {
			triggerDismiss('focus-outside');
		}
	}

	/**
	 * Activate dismissable
	 */
	function activate(target: HTMLElement): void {
		if (state.active) {
			deactivate();
		}

		state.target = target;
		state.active = true;

		// Add to layer stack
		layerStack.push(selfRef);
		state.layerIndex = layerStack.length - 1;

		// Set up event listeners
		clickHandler = handleClick;
		keydownHandler = handleKeydown;
		scrollHandler = handleScroll;
		resizeHandler = handleResize;
		focusinHandler = handleFocusin;

		// Use capture phase for click to handle before other handlers
		document.addEventListener('mousedown', clickHandler, true);
		document.addEventListener('keydown', keydownHandler, true);

		if (currentConfig.closeOnScroll) {
			document.addEventListener('scroll', scrollHandler, true);
		}

		if (currentConfig.closeOnResize) {
			window.addEventListener('resize', resizeHandler);
		}

		if (currentConfig.closeOnFocusOutside) {
			document.addEventListener('focusin', focusinHandler, true);
		}
	}

	/**
	 * Deactivate dismissable
	 */
	function deactivate(): void {
		if (!state.active) return;

		// Remove from layer stack
		const index = layerStack.indexOf(selfRef);
		if (index !== -1) {
			layerStack.splice(index, 1);
		}

		// Remove event listeners
		if (clickHandler) {
			document.removeEventListener('mousedown', clickHandler, true);
			clickHandler = null;
		}

		if (keydownHandler) {
			document.removeEventListener('keydown', keydownHandler, true);
			keydownHandler = null;
		}

		if (scrollHandler) {
			document.removeEventListener('scroll', scrollHandler, true);
			scrollHandler = null;
		}

		if (resizeHandler) {
			window.removeEventListener('resize', resizeHandler);
			resizeHandler = null;
		}

		if (focusinHandler) {
			document.removeEventListener('focusin', focusinHandler, true);
			focusinHandler = null;
		}

		state.target = null;
		state.active = false;
		state.layerIndex = -1;
	}

	/**
	 * Programmatically dismiss
	 */
	function dismiss(): void {
		triggerDismiss('programmatic');
	}

	/**
	 * Add element to exclude list
	 */
	function addExclude(element: HTMLElement): void {
		if (!currentConfig.excludeElements.includes(element)) {
			currentConfig.excludeElements.push(element);
		}
	}

	/**
	 * Remove element from exclude list
	 */
	function removeExclude(element: HTMLElement): void {
		const index = currentConfig.excludeElements.indexOf(element);
		if (index !== -1) {
			currentConfig.excludeElements.splice(index, 1);
		}
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<DismissableConfig>): void {
		const wasActive = state.active;
		const target = state.target;

		// Deactivate to remove old listeners
		if (wasActive) {
			deactivate();
		}

		currentConfig = {
			...currentConfig,
			...newConfig,
			excludeElements: newConfig.excludeElements ?? currentConfig.excludeElements,
			onDismiss: newConfig.onDismiss ?? currentConfig.onDismiss,
			onBeforeDismiss: newConfig.onBeforeDismiss ?? currentConfig.onBeforeDismiss,
		};

		// Reactivate with new config
		if (wasActive && target) {
			activate(target);
		}
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		deactivate();
	}

	// Create self reference
	const selfRef: Dismissable = {
		state,
		activate,
		deactivate,
		dismiss,
		addExclude,
		removeExclude,
		updateConfig,
		destroy,
	};

	return selfRef;
}

/**
 * Get current layer stack depth
 */
export function getDismissableLayerCount(): number {
	return layerStack.length;
}

/**
 * Clear all dismissable layers (for testing)
 */
export function clearDismissableLayers(): void {
	while (layerStack.length > 0) {
		const layer = layerStack.pop();
		layer?.deactivate();
	}
}
