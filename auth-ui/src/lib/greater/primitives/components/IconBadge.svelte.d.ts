import type { Component, Snippet } from 'svelte';
interface Props {
	/**
	 * Icon component to display.
	 */
	icon?: Component;
	/**
	 * Override icon size. Defaults based on badge size.
	 */
	iconSize?: number;
	/**
	 * Size of the badge.
	 */
	size?: 'sm' | 'md' | 'lg' | 'xl';
	/**
	 * Color theme.
	 */
	color?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
	/**
	 * Visual variant.
	 */
	variant?: 'filled' | 'outlined' | 'ghost';
	/**
	 * Shape of the badge.
	 */
	shape?: 'circle' | 'rounded' | 'square';
	/**
	 * Additional CSS classes.
	 */
	class?: string;
	/**
	 * Snippet for icon content (alternative to icon prop).
	 */
	children?: Snippet;
}
/**
 * IconBadge component - A container for icons with consistent shapes, sizes, and colors.
 *
 *
 * @example
 * ```svelte
 * <IconBadge icon={TargetIcon} size="lg" color="primary" />
 * <IconBadge icon={SettingsIcon} variant="outlined" shape="square" />
 * ```
 */
declare const IconBadge: Component<Props, {}, ''>;
type IconBadge = ReturnType<typeof IconBadge>;
export default IconBadge;
//# sourceMappingURL=IconBadge.svelte.d.ts.map
