<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		label: string;
		monospace?: boolean;
		wrap?: boolean;
		class?: string;
		children: Snippet;
		actions?: Snippet;
	}

	let {
		label,
		monospace = false,
		wrap = true,
		class: className = '',
		children,
		actions,
		...restProps
	}: Props = $props();

	const itemClass = $derived(['gr-definition-item', className].filter(Boolean).join(' '));

	const valueClass = $derived(
		[
			'gr-definition-item__value',
			monospace && 'gr-definition-item__value--monospace',
			wrap ? 'gr-definition-item__value--wrap' : 'gr-definition-item__value--nowrap',
		]
			.filter(Boolean)
			.join(' ')
	);
</script>

<div class={itemClass} {...restProps}>
	<dt class="gr-definition-item__label">{label}</dt>
	<dd class="gr-definition-item__content">
		<div class={valueClass}>
			{@render children()}
		</div>
		{#if actions}
			<div class="gr-definition-item__actions">
				{@render actions()}
			</div>
		{/if}
	</dd>
</div>

