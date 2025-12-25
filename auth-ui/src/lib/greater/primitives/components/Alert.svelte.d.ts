import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Alert component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<HTMLDivElement> {
	/**
	 * Visual variant determining color scheme and default icon.
	 * - `error`: Red color scheme, X-circle icon, role="alert"
	 * - `warning`: Yellow/amber color scheme, alert-triangle icon, role="alert"
	 * - `success`: Green color scheme, check-circle icon, role="status"
	 * - `info`: Blue color scheme, info icon, role="status"
	 *
	 * @defaultValue 'info'
	 * @public
	 */
	variant?: 'error' | 'warning' | 'success' | 'info';
	/**
	 * Optional title text displayed prominently above the message content.
	 *
	 * @public
	 */
	title?: string;
	/**
	 * Whether the alert can be dismissed by the user.
	 * When true, shows a close button.
	 *
	 * @defaultValue false
	 * @public
	 */
	dismissible?: boolean;
	/**
	 * Callback function invoked when the alert is dismissed.
	 * Only relevant when dismissible is true.
	 *
	 * @public
	 */
	onDismiss?: () => void;
	/**
	 * Optional label text for an action button.
	 * When provided, displays an action button.
	 *
	 * @public
	 */
	actionLabel?: string;
	/**
	 * Callback function invoked when the action button is clicked.
	 * Only relevant when actionLabel is provided.
	 *
	 * @public
	 */
	onAction?: () => void;
	/**
	 * Custom icon snippet to override the default variant icon.
	 *
	 * @public
	 */
	icon?: Snippet;
	/**
	 * Main alert message content.
	 *
	 * @public
	 */
	children?: Snippet;
	/**
	 * Additional CSS classes to apply to the alert container.
	 *
	 * @public
	 */
	class?: string;
}
/**
 * Alert component - A versatile alert/banner component for displaying error, warning, success, and info messages.
 *
 *
 * @example
 * ```svelte
 * <Alert variant="error" title="Connection Lost" dismissible onDismiss={handleDismiss}>
 * Your session has expired. Please sign in again.
 * </Alert>
 *
 * <Alert variant="success" actionLabel="View Details" onAction={handleView}>
 * Your changes have been saved successfully.
 * </Alert>
 * ```
 */
declare const Alert: import('svelte').Component<Props, {}, ''>;
type Alert = ReturnType<typeof Alert>;
export default Alert;
//# sourceMappingURL=Alert.svelte.d.ts.map
