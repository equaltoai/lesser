<script lang="ts">
	import { setContext, untrack } from 'svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		children?: Snippet;
		prefix?: string;
	}

	let { children, prefix = 'gr' }: Props = $props();

	// Create a counter object that will be shared by all consumers in this tree
	const counter = { current: 0, prefix: untrack(() => prefix) };
	setContext('gr-id-provider', counter);
</script>

{#if children}
	{@render children()}
{/if}
