<!--
Badge component - A versatile badge component for status indicators, labels, and counts.

@component
@example
```svelte
<Badge variant="pill" label="New" color="primary">
  Feature available
</Badge>

<Badge variant="dot" color="success" label="Online" />
```
-->
<script lang="ts">
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

		/**
		 * Whether to show a pulse animation (useful for status indicators).
		 */
		pulse?: boolean;

		/**
		 * Accessibility live region behavior.
		 */
		ariaLive?: 'polite' | 'assertive';
	}

	let {
		variant = 'pill',
		color = 'primary',
		size = 'md',
		label = '',
		description = '',
		class: className = '',
		labelSnippet,
		children,
		pulse = false,
		ariaLive,
		...restProps
	}: Props = $props();

	const badgeClass = $derived(
		[
			'gr-badge',
			`gr-badge--${variant}`,
			`gr-badge--${color}`,
			`gr-badge--${size}`,
			pulse && 'gr-badge--pulse',
			className,
		]
			.filter(Boolean)
			.join(' ')
	);
</script>

<div class={badgeClass} aria-live={ariaLive} {...restProps}>
	{#if variant === 'dot'}
		<span
			class={['gr-badge__dot', pulse && 'gr-badge__dot--pulse'].filter(Boolean).join(' ')}
			aria-hidden="true"
		></span>
	{/if}

	{#if label || labelSnippet}
		<span class="gr-badge__label">
			{#if labelSnippet}
				{@render labelSnippet()}
			{:else}
				{label}
			{/if}
		</span>
	{/if}

	{#if (description || children) && variant === 'pill'}
		<span class="gr-badge__description">
			{#if children}
				{@render children()}
			{:else}
				{description}
			{/if}
		</span>
	{:else if children}
		<!-- Allow children for other variants if needed, though typically they just have a label -->
		<span class="gr-badge__content">
			{@render children()}
		</span>
	{/if}
</div>
