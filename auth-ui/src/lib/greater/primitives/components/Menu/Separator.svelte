<script lang="ts">
	interface Props {
		/** Custom CSS class */
		class?: string;
	}

	let { class: className = '' }: Props = $props();
</script>

<div class="gr-menu-separator {className}" role="separator" aria-orientation="horizontal"></div>

<style>
	.gr-menu-separator {
		height: 1px;
		background-color: var(--gr-color-border, #e5e7eb);
		margin: var(--gr-spacing-xs, 4px) 0;
	}
</style>
