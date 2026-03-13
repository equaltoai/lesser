import type { HTMLButtonAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
/**
 * Button component props interface.
 *
 * @public
 */
interface Props extends Omit<HTMLButtonAttributes, 'type' | 'prefix'> {
	/**
	 * Visual variant of the button.
	 * - `solid`: Primary button with filled background (default)
	 * - `outline`: Secondary button with border
	 * - `ghost`: Tertiary button with no background/border
	 *
	 * @defaultValue 'solid'
	 * @public
	 */
	variant?: 'solid' | 'outline' | 'ghost';
	/**
	 * Size of the button affecting padding and font size.
	 * - `sm`: Small button (2rem height, 0.875rem font)
	 * - `md`: Medium button (2.5rem height, 1rem font) (default)
	 * - `lg`: Large button (3rem height, 1.125rem font)
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	size?: 'sm' | 'md' | 'lg';
	/**
	 * HTML button type attribute.
	 *
	 * @defaultValue 'button'
	 * @public
	 */
	type?: 'button' | 'submit' | 'reset';
	/**
	 * Whether the button is disabled. Disabled buttons cannot be interacted with
	 * and have reduced opacity.
	 *
	 * @defaultValue false
	 * @public
	 */
	disabled?: boolean;
	/**
	 * Whether the button is in a loading state. Loading buttons show a spinner
	 * and are non-interactive.
	 *
	 * @defaultValue false
	 * @public
	 */
	loading?: boolean;
	/**
	 * Controls how the loading spinner is displayed relative to button content.
	 * - `replace-prefix`: Spinner replaces the prefix icon (default)
	 * - `append`: Spinner appears after the button text (in suffix position)
	 * - `prepend`: Spinner appears before everything (including prefix)
	 *
	 * @defaultValue 'replace-prefix'
	 * @public
	 */
	loadingBehavior?: 'replace-prefix' | 'append' | 'prepend';
	/**
	 * Additional CSS classes to apply to the button.
	 *
	 * @public
	 */
	class?: string;
	/**
	 * Main button content snippet.
	 *
	 * @public
	 */
	children?: Snippet;
	/**
	 * Content to display before the main button text (e.g., icon).
	 *
	 * @public
	 */
	prefix?: Snippet;
	/**
	 * Content to display after the main button text (e.g., arrow icon).
	 *
	 * @public
	 */
	suffix?: Snippet;
}
/**
 * Button component - Accessible interactive element with loading states, variants, and full keyboard navigation.
 *
 *
 * @example
 * ```svelte
 * <Button variant="solid" size="md" onclick={handleClick}>
 * Click me
 * </Button>
 *
 * <Button variant="outline" loading loadingBehavior="replace-prefix">
 * {#snippet prefix()}
 *   <Icon name="plus" />
 * {/snippet}
 * Loading...
 * </Button>
 * ```
 */
declare const Button: import('svelte').Component<Props, {}, ''>;
type Button = ReturnType<typeof Button>;
export default Button;
//# sourceMappingURL=Button.svelte.d.ts.map
