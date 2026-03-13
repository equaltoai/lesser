import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
interface Props extends Omit<HTMLAttributes<HTMLElement>, 'role'> {
	src?: string;
	alt?: string;
	name?: string;
	/**
	 * Text label fallback for chat interfaces (e.g., "You", "PAI", "AI")
	 * Used when fallbackMode is 'label'
	 */
	label?: string;
	/**
	 * Icon snippet fallback for avatar
	 * Used when fallbackMode is 'icon'
	 */
	labelIcon?: Snippet;
	/**
	 * Controls which fallback to use when image is not available
	 * - 'initials': Show initials from name prop (default)
	 * - 'label': Show label text
	 * - 'icon': Show labelIcon or default user icon
	 * @default 'initials'
	 */
	fallbackMode?: 'initials' | 'label' | 'icon';
	size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
	shape?: 'circle' | 'square' | 'rounded';
	loading?: boolean;
	status?: 'online' | 'offline' | 'busy' | 'away';
	statusPosition?: 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';
	class?: string;
	fallback?: Snippet;
	role?: string;
}
declare const Avatar: import('svelte').Component<Props, {}, ''>;
type Avatar = ReturnType<typeof Avatar>;
export default Avatar;
//# sourceMappingURL=Avatar.svelte.d.ts.map
