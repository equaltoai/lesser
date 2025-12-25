<!--
  Group of related settings fields
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		label?: string;
		children: Snippet;
		orientation?: 'vertical' | 'horizontal';
	}

	let { label, children, orientation = 'vertical' }: Props = $props();
</script>

<div class="settings-group" data-orientation={orientation}>
	{#if label}
		<h4 class="settings-group__label">{label}</h4>
	{/if}
	<div class="settings-group__content">
		{@render children()}
	</div>
</div>

<style>
	.settings-group {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	.settings-group__label {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--gr-color-text);
		margin: 0;
	}
	.settings-group__content {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	.settings-group[data-orientation='horizontal'] .settings-group__content {
		flex-direction: row;
		gap: 1rem;
		flex-wrap: wrap;
	}
</style>
