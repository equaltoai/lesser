/**
 * Skeleton Primitive
 *
 * Headless primitive for skeleton loading states with animation control.
 * Provides behavior for displaying loading placeholders with various shapes and animations.
 *
 * @module primitives/skeleton
 */

/**
 * Skeleton configuration
 */
export interface SkeletonConfig {
	/**
	 * Whether to show the skeleton
	 */
	loading?: boolean;

	/**
	 * Animation type
	 */
	animation?: 'pulse' | 'wave' | 'none';

	/**
	 * Variant/shape type
	 */
	variant?: 'text' | 'circular' | 'rectangular' | 'rounded';

	/**
	 * Width (CSS value)
	 */
	width?: string | number;

	/**
	 * Height (CSS value)
	 */
	height?: string | number;

	/**
	 * Number of rows (for text variant)
	 */
	rows?: number;

	/**
	 * Delay before showing skeleton (ms)
	 */
	delay?: number;

	/**
	 * Animation duration (ms)
	 */
	duration?: number;

	/**
	 * Custom class name
	 */
	class?: string;

	/**
	 * Animation complete handler
	 */
	onAnimationComplete?: () => void;

	/**
	 * Cleanup handler
	 */
	onDestroy?: () => void;
}

/**
 * Skeleton state
 */
export interface SkeletonState {
	/**
	 * Whether to show the skeleton
	 */
	loading: boolean;

	/**
	 * Whether the skeleton is visible (after delay)
	 */
	visible: boolean;

	/**
	 * Animation type
	 */
	animation: 'pulse' | 'wave' | 'none';

	/**
	 * Variant/shape type
	 */
	variant: 'text' | 'circular' | 'rectangular' | 'rounded';

	/**
	 * Width
	 */
	width: string | number;

	/**
	 * Height
	 */
	height: string | number;

	/**
	 * Number of rows
	 */
	rows: number;

	/**
	 * Animation duration
	 */
	duration: number;

	/**
	 * Custom class name
	 */
	class: string;
}

/**
 * Skeleton actions
 */
export interface SkeletonActions {
	/**
	 * Svelte action for skeleton element
	 */
	skeleton: (node: HTMLElement) => { destroy: () => void };
}

/**
 * Skeleton helpers
 */
export interface SkeletonHelpers {
	/**
	 * Show the skeleton
	 */
	show: () => void;

	/**
	 * Hide the skeleton
	 */
	hide: () => void;

	/**
	 * Toggle the skeleton
	 */
	toggle: () => void;

	/**
	 * Set loading state
	 */
	setLoading: (loading: boolean) => void;
}

/**
 * Skeleton context
 */
export interface Skeleton {
	/**
	 * Current state
	 */
	state: SkeletonState;

	/**
	 * Actions to apply to DOM elements
	 */
	actions: SkeletonActions;

	/**
	 * Helper functions
	 */
	helpers: SkeletonHelpers;
}

/**
 * Create a skeleton
 *
 * @param config - Skeleton configuration
 * @returns Skeleton context
 */
export function createSkeleton(config: SkeletonConfig = {}): Skeleton {
	const {
		loading = true,
		animation = 'pulse',
		variant = 'text',
		width = '100%',
		height = variant === 'text' ? '1em' : '100%',
		rows = 1,
		delay = 0,
		duration = 1500,
		class: className = '',
		onAnimationComplete,
		onDestroy,
	} = config;

	// Internal state
	let skeletonElement: HTMLElement | null = null;
	let delayTimeout: ReturnType<typeof setTimeout> | null = null;

	// Reactive state (works in both Svelte and test environments)
	const internalState: SkeletonState = {
		loading,
		visible: delay === 0 && loading,
		animation,
		variant,
		width,
		height,
		rows,
		duration,
		class: className,
	};

	// Create proxy for reactivity
	const state = new Proxy(internalState, {
		set(target, prop: keyof SkeletonState, value) {
			target[prop] = value as never;
			updateDOM();
			return true;
		},
	});

	function updateDOM() {
		if (skeletonElement) {
			skeletonElement.setAttribute('data-visible', String(state.visible));
			skeletonElement.setAttribute('data-animation', state.animation);
			skeletonElement.setAttribute('data-variant', state.variant);
		}
	}

	// Handle delay
	if (delay > 0 && loading) {
		delayTimeout = setTimeout(() => {
			if (state.loading) {
				state.visible = true;
			}
		}, delay);
	}

	/**
	 * Handle animation end
	 */
	function handleAnimationEnd() {
		onAnimationComplete?.();
	}

	/**
	 * Skeleton action
	 */
	function skeleton(node: HTMLElement) {
		skeletonElement = node;

		// Set data attributes for styling
		node.setAttribute('data-animation', state.animation);
		node.setAttribute('data-variant', state.variant);
		node.setAttribute('data-visible', state.visible.toString());

		// Set style properties
		node.style.setProperty(
			'--skeleton-width',
			typeof state.width === 'number' ? `${state.width}px` : state.width
		);
		node.style.setProperty(
			'--skeleton-height',
			typeof state.height === 'number' ? `${state.height}px` : state.height
		);
		node.style.setProperty('--skeleton-duration', `${state.duration}ms`);

		// Add animation end listener
		if (state.animation !== 'none') {
			node.addEventListener('animationiteration', handleAnimationEnd);
		}

		return {
			destroy() {
				if (delayTimeout) {
					clearTimeout(delayTimeout);
					delayTimeout = null;
				}

				if (skeletonElement) {
					if (state.animation !== 'none') {
						node.removeEventListener('animationiteration', handleAnimationEnd);
					}
					skeletonElement = null;
				}

				onDestroy?.();
			},
		};
	}

	/**
	 * Show helper
	 */
	function show() {
		state.loading = true;

		if (delay > 0) {
			if (delayTimeout) {
				clearTimeout(delayTimeout);
			}
			delayTimeout = setTimeout(() => {
				if (state.loading) {
					state.visible = true;
				}
			}, delay);
		} else {
			state.visible = true;
		}
	}

	/**
	 * Hide helper
	 */
	function hide() {
		state.loading = false;
		state.visible = false;

		if (delayTimeout) {
			clearTimeout(delayTimeout);
			delayTimeout = null;
		}
	}

	/**
	 * Toggle helper
	 */
	function toggle() {
		if (state.loading) {
			hide();
		} else {
			show();
		}
	}

	/**
	 * Set loading helper
	 */
	function setLoading(newLoading: boolean) {
		if (newLoading) {
			show();
		} else {
			hide();
		}
	}

	return {
		state,
		actions: {
			skeleton,
		},
		helpers: {
			show,
			hide,
			toggle,
			setLoading,
		},
	};
}
