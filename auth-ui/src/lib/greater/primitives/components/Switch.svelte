<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLInputAttributes, 'type' | 'checked'> {
		checked?: boolean;
		disabled?: boolean;
		class?: string;
		label?: string;
		onchange?: (checked: boolean) => void;
	}

	let {
		checked = $bindable(false),
		disabled = false,
		class: className = '',
		id,
		name,
		label,
		onchange,
		...restProps
	}: Props = $props();

	function handleChange(event: Event) {
		const target = event.target as HTMLInputElement;
		checked = target.checked;
		onchange?.(checked);
	}
</script>

<label
	class={`gr-switch ${className}`}
	class:gr-switch--checked={checked}
	class:gr-switch--disabled={disabled}
>
	<input
		type="checkbox"
		bind:checked
		{disabled}
		{id}
		{name}
		class="gr-switch__input"
		onchange={handleChange}
		{...restProps}
	/>
	<span class="gr-switch__slider"></span>
	{#if label}
		<span class="gr-switch__label">{label}</span>
	{/if}
</label>
