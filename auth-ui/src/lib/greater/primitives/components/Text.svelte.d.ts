import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Text component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<
	HTMLParagraphElement | HTMLSpanElement | HTMLDivElement | HTMLLabelElement
> {
	/**
	 * HTML element to render.
	 * - `p`: Paragraph (default)
	 * - `span`: Inline span
	 * - `div`: Block div
	 * - `label`: Label element
	 *
	 * @defaultValue 'p'
	 * @public
	 */
	as?: 'p' | 'span' | 'div' | 'label';
	/**
	 * Text size.
	 * - `xs`: 0.75rem
	 * - `sm`: 0.875rem
	 * - `base`: 1rem (default)
	 * - `lg`: 1.125rem
	 * - `xl`: 1.25rem
	 * - `2xl`: 1.5rem
	 *
	 * @defaultValue 'base'
	 * @public
	 */
	size?: 'xs' | 'sm' | 'base' | 'lg' | 'xl' | '2xl';
	/**
	 * Font weight.
	 * - `normal`: 400 (default)
	 * - `medium`: 500
	 * - `semibold`: 600
	 * - `bold`: 700
	 *
	 * @defaultValue 'normal'
	 * @public
	 */
	weight?: 'normal' | 'medium' | 'semibold' | 'bold';
	/**
	 * Text color variant.
	 * - `primary`: Primary text color (default)
	 * - `secondary`: Muted text color
	 * - `tertiary`: Most muted text color
	 * - `success`: Success/positive color
	 * - `warning`: Warning color
	 * - `error`: Error/danger color
	 *
	 * @defaultValue 'primary'
	 * @public
	 */
	color?: 'primary' | 'secondary' | 'tertiary' | 'success' | 'warning' | 'error';
	/**
	 * Text alignment.
	 *
	 * @defaultValue 'left'
	 * @public
	 */
	align?: 'left' | 'center' | 'right' | 'justify';
	/**
	 * Whether text should truncate with ellipsis.
	 *
	 * @defaultValue false
	 * @public
	 */
	truncate?: boolean;
	/**
	 * Number of lines before truncating (requires truncate=true).
	 * If not set, single-line truncation is used.
	 *
	 * @public
	 */
	lines?: number;
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
 * Text component - Paragraph and inline text component with size, weight, and color variants.
 *
 *
 * @example
 * ```svelte
 * <Text size="lg" color="secondary">Some text content</Text>
 * ```
 */
declare const Text: import('svelte').Component<Props, {}, ''>;
type Text = ReturnType<typeof Text>;
export default Text;
//# sourceMappingURL=Text.svelte.d.ts.map
