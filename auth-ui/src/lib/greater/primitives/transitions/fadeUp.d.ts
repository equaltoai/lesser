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
 * Fade up transition - element fades in while moving upward
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export declare function fadeUp(node: Element, params?: FadeUpParams): TransitionConfig;
export default fadeUp;
//# sourceMappingURL=fadeUp.d.ts.map
