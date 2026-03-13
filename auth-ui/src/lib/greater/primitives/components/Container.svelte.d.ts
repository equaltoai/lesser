import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Size/max-width preset values.
 * @public
 */
export type ContainerSize = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';
/**
 * Gutter/padding preset values.
 * @public
 */
export type GutterPreset = 'none' | 'sm' | 'md' | 'lg' | 'xl';
/**
 * Container component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<HTMLDivElement> {
	/**
	 * Maximum width constraint.
	 * - `sm`: 640px
	 * - `md`: 768px
	 * - `lg`: 1024px (default)
	 * - `xl`: 1280px
	 * - `2xl`: 1536px
	 * - `full`: 100% (no constraint)
	 *
	 * @defaultValue 'lg'
	 * @public
	 */
	maxWidth?: ContainerSize;
	/**
	 * Alias for maxWidth. Provides consistent API with other components.
	 * If both size and maxWidth are provided, size takes precedence.
	 *
	 * @public
	 */
	size?: ContainerSize;
	/**
	 * Horizontal padding.
	 * - `false`: No padding
	 * - `true`: Default padding (1rem)
	 * - `sm`: 0.75rem
	 * - `md`: 1rem
	 * - `lg`: 1.5rem
	 *
	 * @defaultValue true
	 * @public
	 * @deprecated Use `gutter` instead for more explicit naming
	 */
	padding?: boolean | 'sm' | 'md' | 'lg';
	/**
	 * Horizontal padding (gutter) control.
	 * Only preset values are supported for CSP compliance.
	 *
	 * Preset values:
	 * - `none`: No padding
	 * - `sm`: 0.75rem
	 * - `md`: 1rem (default)
	 * - `lg`: 1.5rem
	 * - `xl`: 2rem
	 *
	 * For custom gutter values, use the `class` prop with external CSS
	 * to set the `--gr-container-custom-gutter` CSS variable.
	 * Takes precedence over `padding` prop.
	 *
	 * @public
	 */
	gutter?: GutterPreset;
	/**
	 * Center content horizontally.
	 *
	 * @defaultValue true
	 * @public
	 */
	centered?: boolean;
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
 * Container component - Max-width wrapper for content centering.
 *
 *
 * @example
 * ```svelte
 * <Container maxWidth="lg" padding="md" centered>
 * <h1>Page Title</h1>
 * <p>Page content...</p>
 * </Container>
 * ```
 *
 * @example Using size prop (alias for maxWidth)
 * ```svelte
 * <Container size="xl" gutter="lg">
 * <h1>Page Title</h1>
 * </Container>
 * ```
 *
 * @example Custom gutter via external CSS
 * ```svelte
 * <Container size="lg" class="custom-gutter">
 * <h1>Custom Padding</h1>
 * </Container>
 *
 * <style>
 *   :global(.custom-gutter) {
 *     --gr-container-custom-gutter: 2rem;
 *   }
 * </style>
 * ```
 */
declare const Container: import('svelte').Component<Props, {}, ''>;
type Container = ReturnType<typeof Container>;
export default Container;
//# sourceMappingURL=Container.svelte.d.ts.map
