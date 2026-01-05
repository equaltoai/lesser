<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		/** Custom CSS class */
		class?: string;
		/** Header content */
		children: Snippet;
	}

	let { class: className = '', children }: Props = $props();
</script>

<div class="gr-menu-header {className}" role="presentation">
	{@render children()}
</div>

<style>
	.gr-menu-header {
		padding: var(--gr-spacing-sm, 8px) var(--gr-spacing-md, 12px);
		font-size: var(--gr-font-size-sm, 0.875rem);
		color: var(--gr-color-text-secondary, #6b7280);
		border-bottom: 1px solid var(--gr-color-border, #e5e7eb);
		margin-bottom: var(--gr-spacing-xs, 4px);
	}
</style>
