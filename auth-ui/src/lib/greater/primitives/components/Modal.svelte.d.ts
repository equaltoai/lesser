import type { Snippet } from 'svelte';
/**
 * Modal component props interface.
 *
 * @public
 */
interface Props {
	/**
	 * Controls whether the modal is open or closed. This is bindable.
	 *
	 * @defaultValue false
	 * @public
	 */
	open?: boolean;
	/**
	 * Title text to display in the modal header.
	 *
	 * @public
	 */
	title?: string;
	/**
	 * Size of the modal affecting width.
	 * - `sm`: 20rem width
	 * - `md`: 28rem width (default)
	 * - `lg`: 40rem width
	 * - `xl`: 56rem width
	 * - `full`: Full viewport minus margins
	 *
	 * @defaultValue 'md'
	 * @public
	 */
	size?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
	/**
	 * Whether the modal can be closed by pressing the Escape key.
	 *
	 * @defaultValue true
	 * @public
	 */
	closeOnEscape?: boolean;
	/**
	 * Whether clicking the backdrop (outside modal content) closes the modal.
	 *
	 * @defaultValue true
	 * @public
	 */
	closeOnBackdrop?: boolean;
	/**
	 * Whether to prevent scrolling on the body when modal is open.
	 *
	 * @defaultValue true
	 * @public
	 */
	preventScroll?: boolean;
	/**
	 * Additional CSS classes to apply to the modal.
	 *
	 * @public
	 */
	class?: string;
	/**
	 * Main modal body content snippet.
	 *
	 * @public
	 */
	children?: Snippet;
	/**
	 * Custom header content snippet. If provided, overrides the title prop.
	 *
	 * @public
	 */
	header?: Snippet;
	/**
	 * Footer content snippet for action buttons.
	 *
	 * @public
	 */
	footer?: Snippet;
	/**
	 * Callback fired when the modal closes.
	 *
	 * @public
	 */
	onClose?: () => void;
	/**
	 * Callback fired when the modal opens.
	 *
	 * @public
	 */
	onOpen?: () => void;
}
/**
 * Modal component - Accessible dialog with focus management, backdrop handling, and scroll locking.
 *
 *
 * @example
 * ```svelte
 * <Modal bind:open title="Settings" size="md" closeOnEscape>
 * <p>Modal content goes here</p>
 *
 * {#snippet footer()}
 *   <Button onclick={() => open = false}>Close</Button>
 * {/snippet}
 * </Modal>
 * ```
 */
declare const Modal: import('svelte').Component<Props, {}, 'open'>;
type Modal = ReturnType<typeof Modal>;
export default Modal;
//# sourceMappingURL=Modal.svelte.d.ts.map
