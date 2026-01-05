<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	interface Props extends HTMLAttributes<HTMLDListElement> {
		density?: 'sm' | 'md';
		dividers?: boolean;
		class?: string;
		children: Snippet;
	}

	let {
		density = 'md',
		dividers = true,
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	const listClass = $derived(
		[
			'gr-definition-list',
			`gr-definition-list--density-${density}`,
			dividers && 'gr-definition-list--dividers',
			className,
		]
			.filter(Boolean)
			.join(' ')
	);
</script>

<dl class={listClass} {...restProps}>
	{@render children()}
</dl>

