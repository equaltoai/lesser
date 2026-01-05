<!--
ListItem component - A single item within a List component.
Automatically inherits icon and styling from parent List.

@component
@example
```svelte
<ListItem>Feature description</ListItem>
<ListItem icon={CustomIcon}>Overridden icon</ListItem>
```
-->
<script lang="ts">
	import { getContext, type Component, type Snippet } from 'svelte';

	interface Props {
		/**
		 * Icon to display for this item (overrides List icon).
		 */
		icon?: Component;

		/**
		 * Color of the icon (overrides List iconColor).
		 */
		iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Item content.
		 */
		children: Snippet;
	}

	let { icon, iconColor, class: className = '', children, ...restProps }: Props = $props();

	const listContext =
		getContext<{
			icon?: Component;
			iconColor?: string;
			iconSize?: number;
		}>('list-context') || {};

	const displayIcon = $derived(icon || listContext.icon);
	const displayColor = $derived(iconColor || listContext.iconColor || 'primary');
	const displaySize = $derived(listContext.iconSize || 20);

	const iconClass = $derived(`gr-list-item__icon gr-list-item__icon--${displayColor}`);
</script>

<li class="gr-list-item {className}" {...restProps}>
	{#if displayIcon}
		{@const Icon = displayIcon}
		<span class={iconClass} aria-hidden="true">
			<Icon size={displaySize} />
		</span>
	{/if}
	<div class="gr-list-item__content">
		{@render children()}
	</div>
</li>
