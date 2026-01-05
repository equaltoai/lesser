import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Size type for heading sizes.
 * @public
 */
type HeadingSize = 'xs' | 'sm' | 'base' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl' | '5xl';
/**
 * Responsive size configuration for different breakpoints.
 * Uses mobile-first approach where smaller breakpoints cascade up.
 * @public
 */
interface ResponsiveSize {
	/** Size for small screens (≥640px) */
	sm?: HeadingSize;
	/** Size for medium screens (≥768px) */
	md?: HeadingSize;
	/** Size for large screens (≥1024px) */
	lg?: HeadingSize;
	/** Size for extra-large screens (≥1280px) */
	xl?: HeadingSize;
}
/**
 * Heading component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<HTMLHeadingElement> {
	/**
	 * Semantic heading level (required for accessibility).
	 * Maps to <h1> through <h6> elements.
	 *
	 * @required
	 * @public
	 */
	level: 1 | 2 | 3 | 4 | 5 | 6;
	/**
	 * Visual size (can differ from semantic level).
	 * - `xs`: 0.75rem
	 * - `sm`: 0.875rem
	 * - `base`: 1rem
	 * - `lg`: 1.125rem
	 * - `xl`: 1.25rem
	 * - `2xl`: 1.5rem
	 * - `3xl`: 1.875rem
	 * - `4xl`: 2.25rem
	 * - `5xl`: 3rem
	 *
	 * @defaultValue Maps to level (h1=5xl, h2=4xl, h3=3xl, h4=2xl, h5=xl, h6=lg)
	 * @public
	 */
	size?: HeadingSize;
	/**
	 * Responsive size configuration for different breakpoints.
	 * Uses mobile-first approach. When set, overrides the `size` prop
	 * at specified breakpoints.
	 *
	 * @example
	 * ```svelte
	 * <Heading level={1} responsiveSize={{ sm: '2xl', md: '3xl', lg: '4xl', xl: '5xl' }}>
	 *   Hero Title
	 * </Heading>
	 * ```
	 *
	 * @public
	 */
	responsiveSize?: ResponsiveSize;
	/**
	 * Enable fluid typography using CSS clamp().
	 * When true, font-size smoothly scales between breakpoints.
	 * Uses predefined clamp ranges based on the size prop.
	 *
	 * Default fluid ranges:
	 * - `5xl`: clamp(2rem, 5vw + 1rem, 3rem)
	 * - `4xl`: clamp(1.75rem, 4vw + 0.75rem, 2.25rem)
	 * - `3xl`: clamp(1.5rem, 3vw + 0.5rem, 1.875rem)
	 * - `2xl`: clamp(1.25rem, 2.5vw + 0.5rem, 1.5rem)
	 * - `xl`: clamp(1.125rem, 2vw + 0.5rem, 1.25rem)
	 *
	 * @defaultValue false
	 * @public
	 */
	fluid?: boolean;
	/**
	 * Font weight.
	 * - `normal`: 400
	 * - `medium`: 500
	 * - `semibold`: 600
	 * - `bold`: 700 (default for headings)
	 *
	 * @defaultValue 'bold'
	 * @public
	 */
	weight?: 'normal' | 'medium' | 'semibold' | 'bold';
	/**
	 * Text alignment.
	 *
	 * @defaultValue 'left'
	 * @public
	 */
	align?: 'left' | 'center' | 'right';
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
 * Heading component - Semantic heading with consistent typography.
 *
 *
 * @example
 * ```svelte
 * <Heading level={1} align="center">Page Title</Heading>
 * ```
 *
 * @example Responsive sizing
 * ```svelte
 * <Heading level={1} responsiveSize={{ sm: '2xl', md: '3xl', lg: '4xl', xl: '5xl' }}>
 * Responsive Hero Title
 * </Heading>
 * ```
 *
 * @example Fluid typography
 * ```svelte
 * <Heading level={1} size="5xl" fluid>
 * Smoothly Scaling Title
 * </Heading>
 * ```
 */
declare const Heading: import('svelte').Component<Props, {}, ''>;
type Heading = ReturnType<typeof Heading>;
export default Heading;
//# sourceMappingURL=Heading.svelte.d.ts.map
