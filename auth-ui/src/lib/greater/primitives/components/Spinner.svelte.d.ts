import type { HTMLAttributes } from 'svelte/elements';
/**
 * Spinner component props interface.
 *
 * @public
 */
interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'role'> {
	/**
	 * Size of the spinner.
	 * - `xs`: 12px
	 * - `sm`: 16px
	 * - `md`: 24px (default)
	 * - `lg`: 32px
	 * - `xl`: 48px
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
	/**
	 * Color of the spinner.
	 * - `primary`: Uses primary theme color
	 * - `current`: Inherits color from parent (currentColor)
	 * - `white`: White color for dark backgrounds
	 * - `gray`: Neutral gray color
	 *
	 * @defaultValue 'primary'
	 * @public
	 */
	color?: 'primary' | 'current' | 'white' | 'gray';
	/**
	 * Accessible label for screen readers.
	 *
	 * @defaultValue 'Loading'
	 * @public
	 */
	label?: string;
	/**
	 * Additional CSS classes to apply.
	 *
	 * @public
	 */
	class?: string;
}
/**
 * Spinner component - Accessible loading indicator with configurable size and color.
 *
 *
 * @example
 * ```svelte
 * <Spinner size="md" color="primary" label="Loading content" />
 *
 * <Spinner size="lg" color="current" />
 * ```
 */
declare const Spinner: import('svelte').Component<Props, {}, ''>;
type Spinner = ReturnType<typeof Spinner>;
export default Spinner;
//# sourceMappingURL=Spinner.svelte.d.ts.map
