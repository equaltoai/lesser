import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * LoadingState component props interface.
 *
 * @public
 */
interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'role'> {
	/**
	 * Size of the spinner.
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
	/**
	 * Optional message to display below the spinner.
	 *
	 * @public
	 */
	message?: string;
	/**
	 * Whether to display as a fullscreen overlay.
	 *
	 * @defaultValue false
	 * @public
	 */
	fullscreen?: boolean;
	/**
	 * Accessible label for the spinner.
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
	/**
	 * Custom content to display instead of or alongside the spinner.
	 *
	 * @public
	 */
	children?: Snippet;
}
/**
 * LoadingState component - Wrapper for Spinner with message and fullscreen overlay support.
 *
 *
 * @example
 * ```svelte
 * <LoadingState message="Loading your data..." />
 *
 * <LoadingState fullscreen size="lg">
 * <p>Custom loading content</p>
 * </LoadingState>
 * ```
 */
declare const LoadingState: import('svelte').Component<Props, {}, ''>;
type LoadingState = ReturnType<typeof LoadingState>;
export default LoadingState;
//# sourceMappingURL=LoadingState.svelte.d.ts.map
