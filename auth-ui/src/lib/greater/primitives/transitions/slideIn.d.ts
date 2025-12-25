/**
 * slideIn Svelte Transition
 * Horizontal slide animation for enter/exit transitions
 *
 * @example
 * ```svelte
 * <!-- Slide from right -->
 * <div transition:slideIn={{ x: 100 }}>Content</div>
 *
 * <!-- Slide from left -->
 * <div transition:slideIn={{ x: -100 }}>Content</div>
 * ```
 */
import type { TransitionConfig } from 'svelte/transition';
export interface SlideInParams {
	/** Animation duration in milliseconds */
	duration?: number;
	/** Animation delay in milliseconds */
	delay?: number;
	/** CSS easing function */
	easing?: (t: number) => number;
	/** Horizontal distance to slide (positive = from right, negative = from left) */
	x?: number | string;
	/** Whether to also fade opacity */
	opacity?: boolean;
}
/**
 * Slide in transition - element slides horizontally into view
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export declare function slideIn(node: Element, params?: SlideInParams): TransitionConfig;
export default slideIn;
//# sourceMappingURL=slideIn.d.ts.map
