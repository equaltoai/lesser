<script lang="ts">
	export interface SelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}

	interface Props {
		value?: string;
		options: SelectOption[];
		placeholder?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		name?: string;
		onchange?: (value: string) => void;
	}

	let {
		value = $bindable(''),
		options,
		placeholder,
		disabled = false,
		required = false,
		class: className = '',
		id,
		name,
		onchange,
	}: Props = $props();

	function handleChange(event: Event) {
		const target = event.target as HTMLSelectElement;
		value = target.value;
		if (onchange) {
			onchange(value);
		}
	}
</script>

<select
	bind:value
	{disabled}
	{required}
	{id}
	{name}
	class={`gr-select ${className}`}
	onchange={handleChange}
>
	{#if placeholder}
		<option value="" disabled selected={!value}>{placeholder}</option>
	{/if}
	{#each options as option (option.value)}
		<option value={option.value} disabled={option.disabled}>
			{option.label}
		</option>
	{/each}
</select>
