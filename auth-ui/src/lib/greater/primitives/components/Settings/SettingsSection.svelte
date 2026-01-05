<!--
  Container for a logical group of settings with header
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		description?: string;
		icon?: Snippet;
		children: Snippet;
	}

	let { title, description, icon, children }: Props = $props();
</script>

<div class="settings-section">
	<div class="settings-section__header">
		{#if icon}
			<div class="settings-section__icon">
				{@render icon()}
			</div>
		{/if}
		<div class="settings-section__title-group">
			<h3 class="settings-section__title">{title}</h3>
			{#if description}
				<p class="settings-section__description">{description}</p>
			{/if}
		</div>
	</div>
	<div class="settings-section__content">
		{@render children()}
	</div>
</div>

<style>
	.settings-section {
		margin-bottom: 2rem;
		background: var(--gr-color-surface);
		border: 1px solid var(--gr-color-border);
		border-radius: var(--gr-radius-md);
		overflow: hidden;
	}
	.settings-section__header {
		padding: 1.5rem;
		border-bottom: 1px solid var(--gr-color-border);
		display: flex;
		align-items: center;
		gap: 1rem;
		background: var(--gr-color-surface-subtle);
	}
	.settings-section__title {
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--gr-color-text);
		margin: 0;
	}
	.settings-section__description {
		font-size: 0.875rem;
		color: var(--gr-color-text-muted);
		margin: 0.25rem 0 0;
	}
	.settings-section__content {
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}
</style>
