<!--
Alert component - A versatile alert/banner component for displaying error, warning, success, and info messages.

@component
@example
```svelte
<Alert variant="error" title="Connection Lost" dismissible onDismiss={handleDismiss}>
  Your session has expired. Please sign in again.
</Alert>

<Alert variant="success" actionLabel="View Details" onAction={handleView}>
  Your changes have been saved successfully.
</Alert>
```
-->
<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';

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

	let {
		variant = 'info',
		title,
		dismissible = false,
		onDismiss,
		actionLabel,
		onAction,
		icon,
		children,
		class: className = '',
		style: _style,
		...restProps
	}: Props = $props();

	// Track visibility for dismissible alerts
	let visible = $state(true);

	// Compute ARIA role based on variant
	const ariaRole = $derived(variant === 'error' || variant === 'warning' ? 'alert' : 'status');

	// Compute alert classes
	const alertClass = $derived(() => {
		const classes = [
			'gr-alert',
			`gr-alert--${variant}`,
			dismissible && 'gr-alert--dismissible',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Handle dismiss action
	function handleDismiss() {
		visible = false;
		onDismiss?.();
	}

	// Handle keyboard events for dismiss
	function handleKeydown(event: KeyboardEvent) {
		if (dismissible && event.key === 'Escape') {
			event.preventDefault();
			handleDismiss();
		}
	}

	// Handle action button click
	// Handle action button click
	function handleAction() {
		onAction?.();
	}
</script>

{#if visible}
	<div
		class={alertClass()}
		role={ariaRole}
		aria-live={variant === 'error' || variant === 'warning' ? 'assertive' : 'polite'}
		onkeydown={handleKeydown}
		transition:fade={{ duration: 200 }}
		{...restProps}
	>
		<div class="gr-alert__icon" aria-hidden="true">
			{#if icon}
				{@render icon()}
			{:else if variant === 'error'}
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
					<circle cx="12" cy="12" r="10" />
					<line x1="15" y1="9" x2="9" y2="15" />
					<line x1="9" y1="9" x2="15" y2="15" />
				</svg>
			{:else if variant === 'warning'}
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
					<path
						d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"
					/>
					<line x1="12" y1="9" x2="12" y2="13" />
					<line x1="12" y1="17" x2="12.01" y2="17" />
				</svg>
			{:else if variant === 'success'}
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
					<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
					<polyline points="22 4 12 14.01 9 11.01" />
				</svg>
			{:else}
				<!-- info variant -->
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
					<circle cx="12" cy="12" r="10" />
					<line x1="12" y1="16" x2="12" y2="12" />
					<line x1="12" y1="8" x2="12.01" y2="8" />
				</svg>
			{/if}
		</div>

		<div class="gr-alert__content">
			{#if title}
				<div class="gr-alert__title">{title}</div>
			{/if}
			{#if children}
				<div class="gr-alert__message">
					{@render children()}
				</div>
			{/if}
		</div>

		{#if actionLabel || dismissible}
			<div class="gr-alert__actions">
				{#if actionLabel}
					<button type="button" class="gr-alert__action-button" onclick={handleAction}>
						{actionLabel}
					</button>
				{/if}
				{#if dismissible}
					<button
						type="button"
						class="gr-alert__dismiss-button"
						onclick={handleDismiss}
						aria-label="Dismiss alert"
					>
						<svg
							width="16"
							height="16"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<line x1="18" y1="6" x2="6" y2="18" />
							<line x1="6" y1="6" x2="18" y2="18" />
						</svg>
					</button>
				{/if}
			</div>
		{/if}
	</div>
{/if}
