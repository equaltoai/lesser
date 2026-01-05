import { type CopyResult } from 'src/lib/greater/utils';
interface Props {
	/**
	 * The text to copy to the clipboard.
	 * Takes precedence over targetSelector if both are provided.
	 */
	text?: string;
	/**
	 * CSS selector for the element to copy text from.
	 * Used if text is not provided.
	 */
	targetSelector?: string;
	/**
	 * Visual layout variant of the button content.
	 * - `icon`: Show only the icon (default)
	 * - `text`: Show only the text
	 * - `icon-text`: Show both icon and text
	 */
	variant?: 'icon' | 'text' | 'icon-text';
	/**
	 * The visual style variant of the underlying Button component.
	 * @default 'ghost'
	 */
	buttonVariant?: 'solid' | 'outline' | 'ghost';
	/**
	 * Size of the button.
	 * @default 'md'
	 */
	size?: 'sm' | 'md' | 'lg';
	/**
	 * Duration in milliseconds to show the success state.
	 * @default 2000
	 */
	feedbackDuration?: number;
	/**
	 * Custom labels for the button text.
	 */
	labels?: {
		default?: string;
		success?: string;
		error?: string;
	};
	/**
	 * Optional callback called after copy attempt.
	 */
	onCopy?: (result: CopyResult) => void;
	/**
	 * Additional class names.
	 */
	class?: string;
}
/**
 * CopyButton component - A button that copies text to the clipboard with visual feedback.
 *
 *
 * @example
 * ```svelte
 * <CopyButton text="npm install @equaltoai/greater-components" />
 *
 * <CopyButton
 * targetSelector="#code-block"
 * variant="icon-text"
 * labels={{ default: 'Copy Code', success: 'Copied!' }}
 * />
 * ```
 */
declare const CopyButton: import('svelte').Component<Props, {}, ''>;
type CopyButton = ReturnType<typeof CopyButton>;
export default CopyButton;
//# sourceMappingURL=CopyButton.svelte.d.ts.map
