import type { HTMLAttributes, HTMLAnchorAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Card component props interface.
 *
 * @public
 */
interface Props extends HTMLAttributes<HTMLElement> {
	/**
	 * Visual variant of the card.
	 * - `elevated`: Card with shadow (default)
	 * - `outlined`: Card with border
	 * - `filled`: Card with background fill
	 *
	 * @defaultValue 'elevated'
	 * @public
	 */
	variant?: 'elevated' | 'outlined' | 'filled';
	/**
	 * Internal padding amount.
	 * - `none`: No padding
	 * - `sm`: 0.75rem padding
	 * - `md`: 1rem padding (default)
	 * - `lg`: 1.5rem padding
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	padding?: 'none' | 'sm' | 'md' | 'lg';
	/**
	 * Whether the card is clickable/interactive.
	 * When true, renders as button with hover states.
	 * Cannot be used with href.
	 *
	 * @defaultValue false
	 * @public
	 */
	clickable?: boolean;
	/**
	 * Whether to show hover effects.
	 * Automatically enabled when href or clickable is set.
	 *
	 * @defaultValue false
	 * @public
	 */
	hoverable?: boolean;
	/**
	 * URL to navigate to when card is clicked.
	 * When set, renders the card as an anchor element.
	 * Cannot be used with clickable.
	 *
	 * @public
	 */
	href?: string;
	/**
	 * Target for the link (only applies when href is set).
	 *
	 * @public
	 */
	target?: HTMLAnchorAttributes['target'];
	/**
	 * Rel attribute for the link (only applies when href is set).
	 * Automatically set to 'noopener noreferrer' when target="_blank".
	 *
	 * @public
	 */
	rel?: string;
	/**
	 * Additional CSS classes.
	 *
	 * @public
	 */
	class?: string;
	/**
	 * Header content snippet.
	 *
	 * @public
	 */
	header?: Snippet;
	/**
	 * Footer content snippet.
	 *
	 * @public
	 */
	footer?: Snippet;
	/**
	 * Main content snippet.
	 *
	 * @public
	 */
	children?: Snippet;
}
/**
 * Card component - Content container with elevation, borders, and semantic sections.
 *
 *
 * @example
 * ```svelte
 * <Card variant="elevated" padding="md" clickable>
 * <p>Card content</p>
 *
 * {#snippet footer()}
 *   <Button>Action</Button>
 * {/snippet}
 * </Card>
 * ```
 *
 * @example Link card
 * ```svelte
 * <Card variant="elevated" href="/details" hoverable>
 * <p>Click to navigate</p>
 * </Card>
 * ```
 */
declare const Card: import('svelte').Component<Props, {}, ''>;
type Card = ReturnType<typeof Card>;
export default Card;
//# sourceMappingURL=Card.svelte.d.ts.map
