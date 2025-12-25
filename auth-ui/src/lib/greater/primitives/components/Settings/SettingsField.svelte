<!--
  Individual setting field with label, description, and control
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		label: string;
		description?: string;
		children: Snippet; // The control (Switch, Select, etc.)
		class?: string;
	}

	let { label, description, children, class: className = '' }: Props = $props();
</script>

<div class={`settings-field ${className}`}>
	<div class="settings-field__info">
		<p class="settings-field__label">{label}</p>
		{#if description}
			<p class="settings-field__description">{description}</p>
		{/if}
	</div>
	<div class="settings-field__control">
		{@render children()}
	</div>
</div>

<style>
	.settings-field {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 2rem;
	}
	.settings-field__info {
		flex: 1;
	}
	.settings-field__label {
		display: block;
		font-size: 0.9375rem;
		font-weight: 500;
		color: var(--gr-color-text);
	}
	.settings-field__description {
		font-size: 0.8125rem;
		color: var(--gr-color-text-muted);
		margin: 0.25rem 0 0;
	}
	.settings-field__control {
		flex-shrink: 0;
	}
</style>
