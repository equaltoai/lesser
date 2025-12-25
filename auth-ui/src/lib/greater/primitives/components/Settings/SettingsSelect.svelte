<!--
  Pre-composed select setting (label + Select)
-->
<script lang="ts">
	import Select from '../Select.svelte';
	import SettingsField from './SettingsField.svelte';

	interface Props<T = string> {
		label: string;
		description?: string;
		value: T;
		options: Array<{ value: T; label: string }>;
		disabled?: boolean;
	}

	let { label, description, value = $bindable(), options, disabled = false }: Props = $props();
</script>

<SettingsField {label} {description}>
	<Select bind:value {options} {disabled} />
</SettingsField>
