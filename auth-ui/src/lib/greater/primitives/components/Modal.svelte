<!--
Modal component - Accessible dialog with focus management, backdrop handling, and scroll locking.

@component
@example
```svelte
<Modal bind:open title="Settings" size="md" closeOnEscape>
  <p>Modal content goes here</p>
  
  {#snippet footer()}
    <Button onclick={() => open = false}>Close</Button>
  {/snippet}
</Modal>
```
-->

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { tick } from 'svelte';
	import { useStableId } from 'src/lib/greater/utils';

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
		 * ID for the modal element.
		 *
		 * @public
		 */
		id?: string;

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

	let {
		open = $bindable(false),
		id,
		title,
		size = 'md',
		closeOnEscape = true,
		closeOnBackdrop = true,
		preventScroll = true,
		class: className = '',
		children,
		header,
		footer,
		onClose,
		onOpen,
	}: Props = $props();

	const canUseDocument = typeof document !== 'undefined';
	const canUseWindow = typeof window !== 'undefined';
	const canUseDom = canUseDocument && canUseWindow;

	let dialog: HTMLDialogElement | undefined = $state();
	let firstFocusable: HTMLElement | undefined = $state();
	let lastFocusable: HTMLElement | undefined = $state();
	let previousActiveElement: Element | null = null;
	let mounted = $state(false);

	// Generate unique ID for ARIA labelling
	const stableId = useStableId('modal');
	const modalId = $derived(id || stableId.value || undefined);
	const titleId = $derived(modalId ? `${modalId}-title` : undefined);

	// Compute modal classes
	const modalClass = $derived(() => {
		const classes = ['gr-modal', `gr-modal--${size}`, className].filter(Boolean).join(' ');

		return classes;
	});

	// Watch for open state changes
	$effect(() => {
		if (open && dialog && !dialog.open) {
			openModal();
		} else if (!open && dialog?.open) {
			closeModal();
		}
	});

	// Handle body scroll lock
	$effect(() => {
		if (!(canUseDom && preventScroll && open && mounted)) {
			return;
		}

		const lockCountAttr = 'data-gr-scroll-lock-count';
		const existingCount = Number(document.body.getAttribute(lockCountAttr) || '0');
		const nextCount = existingCount + 1;

		document.body.setAttribute(lockCountAttr, String(nextCount));
		document.body.classList.add('gr-scroll-locked');

		return () => {
			const currentCount = Number(document.body.getAttribute(lockCountAttr) || '0');
			const updatedCount = Math.max(0, currentCount - 1);

			if (updatedCount === 0) {
				document.body.removeAttribute(lockCountAttr);
				document.body.classList.remove('gr-scroll-locked');
				return;
			}

			document.body.setAttribute(lockCountAttr, String(updatedCount));
		};
	});

	// Mount effect
	$effect(() => {
		mounted = true;
	});

	function openModal() {
		if (!dialog || !canUseDom) return;

		previousActiveElement = document.activeElement;
		dialog.showModal();

		// Set up focus trap
		setupFocusTrap();

		// Focus first focusable element or the dialog itself
		tick().then(() => {
			if (firstFocusable) {
				firstFocusable.focus();
			} else {
				dialog?.focus();
			}
		});

		onOpen?.();
	}

	function closeModal() {
		if (!dialog) return;

		dialog.close();

		// Restore focus to previously active element
		if (previousActiveElement && 'focus' in previousActiveElement) {
			(previousActiveElement as HTMLElement).focus();
		}

		onClose?.();
	}

	function setupFocusTrap() {
		if (!dialog) return;

		const focusableElements = dialog.querySelectorAll(
			'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);

		const focusableArray = Array.from(focusableElements)
			.map((element) => element as HTMLElement)
			.filter((element) => {
				const disableableElement = element as HTMLElement & { disabled?: boolean };
				return !disableableElement.disabled && element.tabIndex >= 0;
			});

		firstFocusable = focusableArray[0];
		lastFocusable = focusableArray[focusableArray.length - 1];
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && closeOnEscape) {
			event.preventDefault();
			open = false;
		}

		if (event.key === 'Tab' && canUseDocument) {
			// Trap focus within modal
			if (event.shiftKey) {
				if (document.activeElement === firstFocusable) {
					event.preventDefault();
					lastFocusable?.focus();
				}
			} else {
				if (document.activeElement === lastFocusable) {
					event.preventDefault();
					firstFocusable?.focus();
				}
			}
		}
	}

	function handleBackdropClick(event: MouseEvent) {
		if (closeOnBackdrop && event.target === dialog) {
			open = false;
		}
	}

	function handleClose() {
		open = false;
	}
</script>

{#if mounted}
	<dialog
		bind:this={dialog}
		class={modalClass()}
		aria-modal="true"
		aria-labelledby={title && titleId ? titleId : undefined}
		onkeydown={handleKeydown}
		onclick={handleBackdropClick}
		onclose={handleClose}
	>
		<div class="gr-modal__content">
			{#if header || title}
				<header class="gr-modal__header">
					{#if header}
						{@render header()}
					{:else if title}
						<h2 id={titleId} class="gr-modal__title">{title}</h2>
					{/if}

					<button
						type="button"
						class="gr-modal__close"
						aria-label="Close modal"
						onclick={() => (open = false)}
					>
						<svg
							width="20"
							height="20"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<line x1="18" y1="6" x2="6" y2="18"></line>
							<line x1="6" y1="6" x2="18" y2="18"></line>
						</svg>
					</button>
				</header>
			{/if}

			<main class="gr-modal__body">
				{#if children}
					{@render children()}
				{/if}
			</main>

			{#if footer}
				<footer class="gr-modal__footer">
					{@render footer()}
				</footer>
			{/if}
		</div>
	</dialog>
{/if}
