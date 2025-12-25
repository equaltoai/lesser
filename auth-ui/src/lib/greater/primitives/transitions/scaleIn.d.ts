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
 * Scale in transition - element scales from center
 *
 * @param node - The DOM element to animate
 * @param params - Configuration options
 * @returns TransitionConfig for Svelte
 */
export declare function scaleIn(node: Element, params?: ScaleInParams): TransitionConfig;
export default scaleIn;
//# sourceMappingURL=scaleIn.d.ts.map
