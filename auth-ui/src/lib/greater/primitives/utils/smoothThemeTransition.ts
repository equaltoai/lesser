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

const DEFAULT_OPTIONS: Required<SmoothThemeTransitionOptions> = {
	duration: 200,
	easing: 'ease',
	properties: ['background-color', 'color', 'border-color', 'box-shadow', 'fill', 'stroke'],
	target:
		typeof document !== 'undefined' ? document.documentElement : (null as unknown as HTMLElement),
};

const TRANSITION_CLASS = 'gr-theme-transitioning';

/**
 * Executes a callback (typically a theme change) with smooth CSS transitions
 * applied to color-related properties.
 *
 * @param callback - Function that performs the theme change
 * @param options - Configuration options for the transition
 * @returns Promise that resolves when the transition completes
 */
export async function smoothThemeTransition(
	callback: () => void | Promise<void>,
	options: SmoothThemeTransitionOptions = {}
): Promise<void> {
	// Skip if running on server
	if (typeof document === 'undefined') {
		await callback();
		return;
	}

	const opts = { ...DEFAULT_OPTIONS, ...options };
	const { duration, target } = opts;

	// Check for reduced motion preference
	const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

	if (prefersReducedMotion) {
		// Skip animation for users who prefer reduced motion
		await callback();
		return;
	}

	// Add transition class
	target.classList.add(TRANSITION_CLASS);

	/** @type {unknown} */
	let callbackError;

	try {
		// Execute the theme change callback
		await callback();
	} catch (err) {
		callbackError = err;
	} finally {
		// Wait for transition to complete, then clean up.
		// Note: the transition CSS is shipped in primitives theme.css; no runtime <style> injection.
		await new Promise((resolve) => setTimeout(resolve, duration));
		target.classList.remove(TRANSITION_CLASS);
	}

	if (callbackError) {
		throw callbackError;
	}
}

/**
 * Creates a theme toggle function with smooth transitions built-in
 *
 * @param getTheme - Function that returns the current theme
 * @param setTheme - Function that sets the new theme
 * @param options - Transition options
 * @returns A function that toggles between themes smoothly
 */
export function createSmoothThemeToggle(
	getTheme: () => string,
	setTheme: (theme: string) => void,
	themes: string[] = ['light', 'dark'],
	options: SmoothThemeTransitionOptions = {}
): () => Promise<void> {
	return async () => {
		const currentTheme = getTheme();
		const currentIndex = themes.indexOf(currentTheme);
		const nextIndex = (currentIndex + 1) % themes.length;
		const nextTheme = themes[nextIndex] ?? themes[0] ?? 'light';

		await smoothThemeTransition(() => {
			setTheme(nextTheme);
		}, options);
	};
}

export default smoothThemeTransition;
