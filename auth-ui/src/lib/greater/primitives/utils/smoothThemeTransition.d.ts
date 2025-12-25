/**
 * Smooth Theme Transition Utility
 *
 * Temporarily applies CSS transitions to color properties during theme changes
 * to create a smooth visual transition between themes.
 *
 * @example
 * ```typescript
 * import { smoothThemeTransition } from 'src/lib/greater/primitives/utils';
 *
 * function toggleTheme() {
 *   smoothThemeTransition(() => {
 *     document.documentElement.setAttribute('data-theme', newTheme);
 *   });
 * }
 * ```
 */
export interface SmoothThemeTransitionOptions {
	/** Duration of the transition in milliseconds */
	duration?: number;
	/** CSS easing function */
	easing?: string;
	/** Properties to transition (defaults to color-related properties) */
	properties?: string[];
	/** Target element to apply transition class (defaults to document.documentElement) */
	target?: HTMLElement;
}
/**
 * Executes a callback (typically a theme change) with smooth CSS transitions
 * applied to color-related properties.
 *
 * @param callback - Function that performs the theme change
 * @param options - Configuration options for the transition
 * @returns Promise that resolves when the transition completes
 */
export declare function smoothThemeTransition(
	callback: () => void | Promise<void>,
	options?: SmoothThemeTransitionOptions
): Promise<void>;
/**
 * Creates a theme toggle function with smooth transitions built-in
 *
 * @param getTheme - Function that returns the current theme
 * @param setTheme - Function that sets the new theme
 * @param options - Transition options
 * @returns A function that toggles between themes smoothly
 */
export declare function createSmoothThemeToggle(
	getTheme: () => string,
	setTheme: (theme: string) => void,
	themes?: string[],
	options?: SmoothThemeTransitionOptions
): () => Promise<void>;
export default smoothThemeTransition;
//# sourceMappingURL=smoothThemeTransition.d.ts.map
