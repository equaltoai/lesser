/**
 * fadeDown Svelte Transition
 * Combines opacity fade with downward motion for enter/exit animations
 *
 * @example
 * ```svelte
 * <div transition:fadeDown={{ duration: 300, delay: 100 }}>
 *   Content
 * </div>
 * ```
 */
import type { TransitionConfig } from 'svelte/transition';
export interface FadeDownParams {
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
 * Fade down transition - element fades in while moving downward
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export declare function fadeDown(node: Element, params?: FadeDownParams): TransitionConfig;
export default fadeDown;
//# sourceMappingURL=fadeDown.d.ts.map
