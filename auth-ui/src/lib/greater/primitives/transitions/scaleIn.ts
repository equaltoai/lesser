/**
 * scaleIn Svelte Transition
 * Scale animation from center for enter/exit transitions
 *
 * @example
 * ```svelte
 * <div transition:scaleIn={{ start: 0.9, duration: 200 }}>
 *   Modal content
 * </div>
 * ```
 */

import type { TransitionConfig } from 'svelte/transition';

export interface ScaleInParams {
	/** Animation duration in milliseconds */
	duration?: number;
	/** Animation delay in milliseconds */
	delay?: number;
	/** CSS easing function */
	easing?: (t: number) => number;
	/** Starting scale value (0-1, where 1 is full size) */
	start?: number;
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
 * Scale in transition - element scales from center
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export function scaleIn(node: Element, params: ScaleInParams = {}): TransitionConfig {
	const {
		duration = 250,
		delay = 0,
		easing = decelerateEasing,
		start = 0.95,
		opacity: fadeOpacity = true,
	} = params;

	const style = getComputedStyle(node);
	const currentOpacity = +style.opacity;
	const transform = style.transform === 'none' ? '' : style.transform;

	return {
		delay,
		duration,
		easing,
		css: (t: number) => {
			const scale = start + (1 - start) * t;
			const opacityValue = fadeOpacity ? t * currentOpacity : currentOpacity;
			return `
        opacity: ${opacityValue};
        transform: ${transform} scale(${scale});
      `;
		},
	};
}

export default scaleIn;
