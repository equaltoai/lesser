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
import { waapiTransition } from './waapi.js';

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
 * Default easing function matching --gr-motion-easing-decelerate
 * cubic-bezier(0, 0, 0.2, 1)
 */
function decelerateEasing(t: number): number {
	return 1 - Math.pow(1 - t, 3);
}

/**
 * Slide in transition - element slides horizontally into view
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export function slideIn(node: Element, params: SlideInParams = {}): TransitionConfig {
	const {
		duration = 250,
		delay = 0,
		easing = decelerateEasing,
		x = 16,
		opacity: fadeOpacity = true,
	} = params;

	const style = getComputedStyle(node);
	const currentOpacity = +style.opacity;
	const transform = style.transform === 'none' ? '' : style.transform;
	const xValue = typeof x === 'number' ? x : parseFloat(x);
	const transformPrefix = transform ? `${transform} ` : '';
	const fromOpacity = fadeOpacity ? 0 : currentOpacity;

	return waapiTransition(
		node,
		[
			{ opacity: fromOpacity, transform: `${transformPrefix}translateX(${xValue}px)` },
			{ opacity: currentOpacity, transform: `${transformPrefix}translateX(0)` },
		],
		{ delay, duration, easing }
	);
}

export default slideIn;
