<!--
LoadingState component - Wrapper for Spinner with message and fullscreen overlay support.

@component
@example
```svelte
<LoadingState message="Loading your data..." />

<LoadingState fullscreen size="lg">
  <p>Custom loading content</p>
</LoadingState>
```
-->

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import Spinner from './Spinner.svelte';

	/**
	 * LoadingState component props interface.
	 *
	 * @public
	 */
	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'role'> {
		/**
		 * Size of the spinner.
		 *
		 * @defaultValue 'md'
		 * @public
		 */
		size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';

		/**
		 * Optional message to display below the spinner.
		 *
		 * @public
		 */
		message?: string;

		/**
		 * Whether to display as a fullscreen overlay.
		 *
		 * @defaultValue false
		 * @public
		 */
		fullscreen?: boolean;

		/**
		 * Accessible label for the spinner.
		 *
		 * @defaultValue 'Loading'
		 * @public
		 */
		label?: string;

		/**
		 * Additional CSS classes to apply.
		 *
		 * @public
		 */
		class?: string;

		/**
		 * Custom content to display instead of or alongside the spinner.
		 *
		 * @public
		 */
		children?: Snippet;
	}

	let {
		size = 'md',
		message,
		fullscreen = false,
		label = 'Loading',
		class: className = '',
		children,
		id,
		style: _style,
		...restProps
	}: Props = $props();

	// Compute container classes
	const containerClass = $derived(
		['gr-loading-state', fullscreen && 'gr-loading-state--fullscreen', className]
			.filter(Boolean)
			.join(' ')
	);
</script>

<div class={containerClass} role="status" aria-live="polite" aria-busy="true" {id} {...restProps}>
	<div class="gr-loading-state__content">
		{#if children}
			{@render children()}
		{:else}
			<Spinner {size} color="primary" {label} />
		{/if}

		{#if message}
			<p class="gr-loading-state__message">{message}</p>
		{/if}
	</div>
</div>

<style>
	.gr-loading-state {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--gr-spacing-scale-4, 1rem);
	}

	.gr-loading-state__content {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--gr-spacing-scale-3, 0.75rem);
	}

	.gr-loading-state__message {
		margin: 0;
		font-family: var(--gr-typography-fontFamily-sans);
		font-size: var(--gr-typography-fontSize-sm);
		color: var(--gr-semantic-text-secondary);
		text-align: center;
	}

	/* Fullscreen overlay */
	.gr-loading-state--fullscreen {
		position: fixed;
		inset: 0;
		z-index: var(--gr-zIndex-modal, 1000);
		background-color: var(--gr-semantic-background-overlay, rgba(0, 0, 0, 0.5));
		backdrop-filter: blur(2px);
	}

	.gr-loading-state--fullscreen .gr-loading-state__content {
		background-color: var(--gr-semantic-background-primary);
		padding: var(--gr-spacing-scale-6, 1.5rem);
		border-radius: var(--gr-radii-lg, 0.5rem);
		box-shadow: var(--gr-shadows-elevation-lg);
	}

	/* Reduced motion */
	@media (prefers-reduced-motion: reduce) {
		.gr-loading-state--fullscreen {
			backdrop-filter: none;
		}
	}
</style>
