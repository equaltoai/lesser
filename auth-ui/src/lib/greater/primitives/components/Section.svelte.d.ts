import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Preset spacing values.
 * @public
 */
export type SpacingPreset = 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl';
/**
 * Background variant presets.
 * @public
 */
export type BackgroundPreset = 'default' | 'muted' | 'accent' | 'gradient';
/**
 * Gradient direction options.
 * @public
 */
export type GradientDirection =
	| 'to-top'
	| 'to-bottom'
	| 'to-left'
	| 'to-right'
	| 'to-top-left'
	| 'to-top-right'
	| 'to-bottom-left'
	| 'to-bottom-right';
/**
 * Section component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<HTMLElement> {
	/**
	 * Vertical spacing (margin-top and margin-bottom).
	 * Must be a preset value for CSP compliance.
	 *
	 * Preset values:
	 * - `none`: No spacing
	 * - `sm`: 2rem
	 * - `md`: 4rem (default)
	 * - `lg`: 6rem
	 * - `xl`: 8rem
	 * - `2xl`: 10rem
	 * - `3xl`: 12rem
	 * - `4xl`: 16rem
	 *
	 * For custom spacing, use the `class` prop with external CSS.
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	spacing?: SpacingPreset;
	/**
	 * Horizontal padding.
	 * - `false`: No padding
	 * - `true`: Default padding (1rem)
	 * - `sm`: 0.75rem
	 * - `md`: 1rem
	 * - `lg`: 1.5rem
	 *
	 * @defaultValue false
	 * @public
	 */
	padding?: boolean | 'sm' | 'md' | 'lg';
	/**
	 * Center content horizontally.
	 *
	 * @defaultValue false
	 * @public
	 */
	centered?: boolean;
	/**
	 * Background variant preset.
	 * Must be a preset value for CSP compliance.
	 *
	 * Preset variants:
	 * - `default`: Transparent/inherit
	 * - `muted`: Subtle secondary background
	 * - `accent`: Primary color tinted background
	 * - `gradient`: Gradient background (use with gradientDirection)
	 *
	 * For custom backgrounds, use the `class` prop with external CSS.
	 *
	 * @defaultValue 'default'
	 * @public
	 */
	background?: BackgroundPreset;
	/**
	 * Direction for gradient backgrounds.
	 * Only applies when background="gradient".
	 *
	 * @defaultValue 'to-bottom'
	 * @public
	 */
	gradientDirection?: GradientDirection;
	/**
	 * Additional CSS classes.
	 *
	 * @public
	 */
	class?: string;
	/**
	 * Content snippet.
	 *
	 * @public
	 */
	children?: Snippet;
}
/**
 * Section component - Semantic section wrapper with consistent vertical spacing.
 *
 *
 * @example
 * ```svelte
 * <Section spacing="lg" padding="md" centered>
 * <h2>Section Title</h2>
 * <p>Section content...</p>
 * </Section>
 * ```
 *
 * @example Extended spacing
 * ```svelte
 * <Section spacing="3xl">
 * <h2>Hero Section</h2>
 * </Section>
 * ```
 *
 * @example Custom styling via external CSS
 * ```svelte
 * <Section class="my-custom-section">
 * <h2>Custom Section</h2>
 * </Section>
 * ```
 *
 * @example Background variants
 * ```svelte
 * <Section background="muted">
 * <h2>Muted Background</h2>
 * </Section>
 *
 * <Section background="gradient" gradientDirection="to-bottom-right">
 * <h2>Gradient Background</h2>
 * </Section>
 * ```
 */
declare const Section: import('svelte').Component<Props, {}, ''>;
type Section = ReturnType<typeof Section>;
export default Section;
//# sourceMappingURL=Section.svelte.d.ts.map
