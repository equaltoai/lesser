<!--
Card component - Content container with elevation, borders, and semantic sections.

@component
@example
```svelte
<Card variant="elevated" padding="md" clickable>
  <p>Card content</p>
  
  {#snippet footer()}
    <Button>Action</Button>
  {/snippet}
</Card>
```

@example Link card
```svelte
<Card variant="elevated" href="/details" hoverable>
  <p>Click to navigate</p>
</Card>
```
-->

<script lang="ts">
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

	let {
		variant = 'elevated',
		padding = 'md',
		clickable = false,
		hoverable = false,
		href,
		target,
		rel,
		class: className = '',
		header,
		footer,
		children,
		onclick,
		onkeydown,
		onkeyup,
		role,
		tabindex,
		style: _style,
		...restProps
	}: Props = $props();

	// Determine if card is interactive (link or button)
	const isLink = $derived(!!href);
	const isInteractive = $derived(clickable || isLink);

	// Compute rel attribute for links
	const computedRel = $derived.by(() => {
		if (!isLink) return undefined;
		if (rel) return rel;
		if (target === '_blank') return 'noopener noreferrer';
		return undefined;
	});

	// Compute card classes
	const cardClass = $derived.by(() => {
		const classes = [
			'gr-card',
			`gr-card--${variant}`,
			`gr-card--padding-${padding}`,
			clickable && 'gr-card--clickable',
			isLink && 'gr-card--link',
			(hoverable || isInteractive) && 'gr-card--hoverable',
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});

	// Handle keyboard activation for clickable cards
	function handleKeydown(
		event: KeyboardEvent & { currentTarget: EventTarget & HTMLButtonElement }
	) {
		onkeydown?.(event);

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

		if (event.key === ' ') {
			event.preventDefault();
			event.currentTarget.click();
		}
	}

	function handleClick(event: MouseEvent & { currentTarget: EventTarget & HTMLButtonElement }) {
		if (clickable && onclick) {
			onclick(event);
		}
	}
</script>

{#snippet cardContent()}
	{#if header}
		<div class="gr-card__header">
			{@render header()}
		</div>
	{/if}

	<div class="gr-card__content">
		{#if children}
			{@render children()}
		{/if}
	</div>

	{#if footer}
		<div class="gr-card__footer">
			{@render footer()}
		</div>
	{/if}
{/snippet}

{#if isLink}
	<a class={cardClass} {href} {target} rel={computedRel} {...restProps}>
		{@render cardContent()}
	</a>
{:else if clickable}
	<button
		class={cardClass}
		onclick={handleClick}
		onkeydown={handleKeydown}
		onkeyup={handleKeyup}
		role={role || 'button'}
		tabindex={tabindex ?? 0}
		{...restProps}
	>
		{@render cardContent()}
	</button>
{:else}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div class={cardClass} {role} {tabindex} {...restProps}>
		{@render cardContent()}
	</div>
{/if}
