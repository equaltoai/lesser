import type { Snippet } from 'svelte';
interface Props {
	/**
	 * Gradient preset or 'custom' mode.
	 * - `primary`: Uses primary-600 to primary-400
	 * - `success`: Uses success-600 to success-400
	 * - `warning`: Uses warning-600 to warning-400
	 * - `error`: Uses error-600 to error-400
	 * - `custom`: Uses from/to/via props
	 */
	gradient?: 'primary' | 'success' | 'warning' | 'error' | 'custom';
	/**
	 * Gradient direction (e.g., "to right", "45deg").
	 */
	direction?: string;
	/**
	 * Start color (for custom gradient).
	 */
	from?: string;
	/**
	 * End color (for custom gradient).
	 */
	to?: string;
	/**
	 * Optional middle color (for custom gradient).
	 */
	via?: string;
	/**
	 * Element tag to render.
	 */
	as?: string;
	/**
	 * Additional CSS classes.
	 */
	class?: string;
	/**
	 * Text content.
	 */
	children: Snippet;
}
/**
 * GradientText component - Eye-catching gradient text effect.
 *
 *
 * @example
 * ```svelte
 * <GradientText gradient="primary">Awesome Heading</GradientText>
 * <GradientText gradient="custom" from="#f00" to="#00f">Custom Gradient</GradientText>
 * ```
 */
declare const GradientText: import('svelte').Component<Props, {}, ''>;
type GradientText = ReturnType<typeof GradientText>;
export default GradientText;
//# sourceMappingURL=GradientText.svelte.d.ts.map
