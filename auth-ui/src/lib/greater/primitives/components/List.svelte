<script lang="ts">
	import { setContext, type Component, type Snippet } from 'svelte';

	interface Props {
		icon?: Component;
		iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
		iconSize?: number;
		spacing?: 'sm' | 'md' | 'lg';
		maxWidth?: string | number;
		ordered?: boolean;
		class?: string;
		children: Snippet;
	}

	let {
		icon,
		iconColor = 'primary',
		iconSize = 20,
		spacing = 'md',
		maxWidth,
		ordered = false,
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	// Use getters in context so consumers always get current prop values
	setContext('list-context', {
		get icon() {
			return icon;
		},
		get iconColor() {
			return iconColor;
		},
		get iconSize() {
			return iconSize;
		},
	});

	const listClass = $derived(
		['gr-list', `gr-list--spacing-${spacing}`, className].filter(Boolean).join(' ')
	);

	const style = $derived(
		maxWidth ? `max-width: ${typeof maxWidth === 'number' ? `${maxWidth}px` : maxWidth};` : ''
	);
</script>

{#if ordered}
	<ol class={listClass} {style} {...restProps}>
		{@render children()}
	</ol>
{:else}
	<ul class={listClass} {style} {...restProps}>
		{@render children()}
	</ul>
{/if}
