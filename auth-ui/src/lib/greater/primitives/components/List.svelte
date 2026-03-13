<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { setContext, type Component, type Snippet } from 'svelte';

	type MaxWidthPreset = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';

	interface Props extends HTMLAttributes<HTMLUListElement | HTMLOListElement> {
		icon?: Component;
		iconColor?: 'primary' | 'success' | 'warning' | 'error' | 'gray';
		iconSize?: number;
		spacing?: 'sm' | 'md' | 'lg';
		maxWidth?: MaxWidthPreset;
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
		style: _style,
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

	const listClass = $derived(() =>
		['gr-list', `gr-list--spacing-${spacing}`, maxWidth && `gr-list--max-${maxWidth}`, className]
			.filter(Boolean)
			.join(' ')
	);
</script>

{#if ordered}
	<ol class={listClass()} {...restProps}>
		{@render children()}
	</ol>
{:else}
	<ul class={listClass()} {...restProps}>
		{@render children()}
	</ul>
{/if}
