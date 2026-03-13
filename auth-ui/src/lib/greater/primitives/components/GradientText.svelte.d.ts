import type { Snippet } from 'svelte';
type GradientPreset = 'primary' | 'success' | 'warning' | 'error';
type GradientDirection =
	| 'to-right'
	| 'to-left'
	| 'to-top'
	| 'to-bottom'
	| 'to-top-right'
	| 'to-top-left'
	| 'to-bottom-right'
	| 'to-bottom-left';
interface Props {
	/**
	 * Gradient preset.
	 * - `primary`: Uses primary-600 to primary-400
	 * - `success`: Uses success-600 to success-400
	 * - `warning`: Uses warning-600 to warning-400
	 * - `error`: Uses error-600 to error-400
	 */
	gradient?: GradientPreset;
	/**
	 * Gradient direction preset.
	 */
	direction?: GradientDirection;
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
 * ```
 */
declare const GradientText: import('svelte').Component<Props, {}, ''>;
type GradientText = ReturnType<typeof GradientText>;
export default GradientText;
//# sourceMappingURL=GradientText.svelte.d.ts.map
