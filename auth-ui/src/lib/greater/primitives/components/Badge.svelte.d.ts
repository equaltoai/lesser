import type { Snippet } from 'svelte';
interface Props {
	/**
	 * Visual variant of the badge.
	 * - `pill`: Rounded pill with optional description (default)
	 * - `dot`: Small dot indicator with text
	 * - `outlined`: Border-only badge
	 * - `filled`: Solid background badge
	 */
	variant?: 'pill' | 'dot' | 'outlined' | 'filled';
	/**
	 * Color theme of the badge.
	 */
	color?: 'primary' | 'success' | 'warning' | 'error' | 'info' | 'gray';
	/**
	 * Size of the badge.
	 */
	size?: 'sm' | 'md' | 'lg';
	/**
	 * Main label text.
	 */
	label?: string;
	/**
	 * Secondary description text (primarily for pill variant).
	 */
	description?: string;
	/**
	 * Additional CSS classes.
	 */
	class?: string;
	/**
	 * Custom label content snippet.
	 */
	labelSnippet?: Snippet;
	/**
	 * Description/Content snippet.
	 */
	children?: Snippet;
}
/**
 * Badge component - A versatile badge component for status indicators, labels, and counts.
 *
 *
 * @example
 * ```svelte
 * <Badge variant="pill" label="New" color="primary">
 * Feature available
 * </Badge>
 *
 * <Badge variant="dot" color="success" label="Online" />
 * ```
 */
declare const Badge: import('svelte').Component<Props, {}, ''>;
type Badge = ReturnType<typeof Badge>;
export default Badge;
//# sourceMappingURL=Badge.svelte.d.ts.map
