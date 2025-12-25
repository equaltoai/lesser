<script lang="ts">
	interface Props {
		checked?: boolean;
		disabled?: boolean;
		required?: boolean;
		indeterminate?: boolean;
		class?: string;
		id?: string;
		name?: string;
		value?: string;
		onchange?: (checked: boolean) => void;
	}

	let {
		checked = $bindable(false),
		disabled = false,
		required = false,
		indeterminate = false,
		class: className = '',
		id,
		name,
		value,
		onchange,
	}: Props = $props();

	function handleChange(event: Event) {
		const target = event.target as HTMLInputElement;
		checked = target.checked;
		onchange?.(checked);
	}
</script>

<input
	type="checkbox"
	bind:checked
	bind:indeterminate
	{disabled}
	{required}
	{id}
	{name}
	{value}
	class={`gr-checkbox ${className}`}
	onchange={handleChange}
/>
