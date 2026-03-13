<!--
Button component - Accessible interactive element with loading states, variants, and full keyboard navigation.

@component
@example
```svelte
<Button variant="solid" size="md" onclick={handleClick}>
  Click me
</Button>

<Button variant="outline" loading loadingBehavior="replace-prefix">
  {#snippet prefix()}
    <Icon name="plus" />
  {/snippet}
  Loading...
</Button>
```
-->

<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import Spinner from './Spinner.svelte';

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

	let {
		variant = 'solid',
		size = 'md',
		type = 'button',
		disabled = false,
		loading = false,
		loadingBehavior = 'replace-prefix',
		class: className = '',
		children,
		prefix,
		suffix,
		onclick,
		onkeydown,
		onkeyup,
		style: _style,
		...restProps
	}: Props = $props();

	// Map button size to spinner size
	const spinnerSizeMap: Record<string, 'xs' | 'sm' | 'md'> = {
		sm: 'xs',
		md: 'sm',
		lg: 'sm',
	};

	const spinnerSize = $derived(spinnerSizeMap[size] || 'sm');

	// Compute button classes
	const buttonClass = $derived.by(() => {
		const classes = [
			'gr-button',
			`gr-button--${variant}`,
			`gr-button--${size}`,
			loading && 'gr-button--loading',
			loading && `gr-button--loading-${loadingBehavior}`,
			disabled && 'gr-button--disabled',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Determine visibility of prefix based on loading state and behavior
	const showPrefix = $derived(
		Boolean(prefix) && (!loading || loadingBehavior !== 'replace-prefix')
	);

	// Determine where to show spinner
	const showSpinnerInPrefix = $derived(loading && loadingBehavior === 'replace-prefix');
	const showSpinnerPrepend = $derived(loading && loadingBehavior === 'prepend');
	const showSpinnerAppend = $derived(loading && loadingBehavior === 'append');

	// Handle keyboard activation
	function handleKeydown(
		event: KeyboardEvent & { currentTarget: EventTarget & HTMLButtonElement }
	) {
		onkeydown?.(event);

		if (disabled || loading) return;

		// Ensure keyboard activation works consistently in strict environments/tests.
		// Enter activates on keydown; Space activates on keyup.
		if (event.key === 'Enter') {
			event.preventDefault();
			event.currentTarget.click();
		}

		if (event.key === ' ') {
			event.preventDefault();
		}
	}

	function handleKeyup(event: KeyboardEvent & { currentTarget: EventTarget & HTMLButtonElement }) {
		onkeyup?.(event);

		if (disabled || loading) return;

		if (event.key === ' ') {
			event.preventDefault();
			event.currentTarget.click();
		}
	}

	function handleClick(event: MouseEvent & { currentTarget: EventTarget & HTMLButtonElement }) {
		if (disabled || loading) {
			event.preventDefault();
			event.stopPropagation();
			return;
		}
		onclick?.(event);
	}
</script>

<button
	class={buttonClass}
	{type}
	disabled={disabled || loading}
	aria-disabled={disabled || loading}
	aria-busy={loading}
	tabindex={disabled ? -1 : 0}
	onclick={handleClick}
	onkeydown={handleKeydown}
	onkeyup={handleKeyup}
	{...restProps}
>
	<!-- Prepend spinner (before everything) -->
	{#if showSpinnerPrepend}
		<span class="gr-button__spinner gr-button__spinner--prepend" aria-hidden="true">
			<Spinner size={spinnerSize} color="current" label="Loading" />
		</span>
	{/if}

	<!-- Prefix slot or replace-prefix spinner -->
	{#if showSpinnerInPrefix}
		<span class="gr-button__spinner gr-button__spinner--prefix" aria-hidden="true">
			<Spinner size={spinnerSize} color="current" label="Loading" />
		</span>
	{:else if prefix && showPrefix}
		<span class="gr-button__prefix">
			{@render prefix()}
		</span>
	{/if}

	<!-- Button content -->
	<span class="gr-button__content" class:gr-button__content--loading={loading}>
		{#if children}
			{@render children()}
		{/if}
	</span>

	<!-- Suffix slot -->
	{#if suffix && !showSpinnerAppend}
		<span class="gr-button__suffix">
			{@render suffix()}
		</span>
	{/if}

	<!-- Append spinner (after text, in suffix position) -->
	{#if showSpinnerAppend}
		<span class="gr-button__spinner gr-button__spinner--append" aria-hidden="true">
			<Spinner size={spinnerSize} color="current" label="Loading" />
		</span>
	{/if}

	<!-- Screen reader loading announcement -->
	{#if loading}
		<span class="gr-button__sr-loading" role="status" aria-live="polite"> Loading </span>
	{/if}
</button>
