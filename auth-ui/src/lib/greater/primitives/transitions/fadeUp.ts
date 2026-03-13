/**
 * fadeUp Svelte Transition
 * Combines opacity fade with upward motion for enter/exit animations
 *
 * @example
 * ```svelte
 * <div transition:fadeUp={{ duration: 300, delay: 100 }}>
 *   Content
 * </div>
 * ```
 */

import type { TransitionConfig } from 'svelte/transition';
import { waapiTransition } from './waapi.js';

export interface FadeUpParams {
	/** Animation duration in milliseconds */
	duration?: number;
	/** Animation delay in milliseconds */
	delay?: number;
	/** CSS easing function */
	easing?: (t: number) => number;
	/** Distance to translate in pixels or CSS units */
	y?: number | string;
}

/**
 * Default easing function matching --gr-motion-easing-decelerate
 * cubic-bezier(0, 0, 0.2, 1)
 */
function decelerateEasing(t: number): number {
	return 1 - Math.pow(1 - t, 3);
}

/**
 * Fade up transition - element fades in while moving upward
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export function fadeUp(node: Element, params: FadeUpParams = {}): TransitionConfig {
	const { duration = 250, delay = 0, easing = decelerateEasing, y = 16 } = params;

	const style = getComputedStyle(node);
	const opacity = +style.opacity;
	const transform = style.transform === 'none' ? '' : style.transform;
	const yValue = typeof y === 'number' ? y : parseFloat(y);
	const transformPrefix = transform ? `${transform} ` : '';

	return waapiTransition(
		node,
		[
			{ opacity: 0, transform: `${transformPrefix}translateY(${yValue}px)` },
			{ opacity, transform: `${transformPrefix}translateY(0)` },
		],
		{ delay, duration, easing }
	);
}

export default fadeUp;
