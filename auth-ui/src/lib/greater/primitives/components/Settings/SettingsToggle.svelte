<!--
  Pre-composed toggle setting (label + Switch)
-->
<script lang="ts">
	import Switch from '../Switch.svelte';
	import SettingsField from './SettingsField.svelte';

	interface Props {
		label: string;
		description?: string;
		value: boolean;
		disabled?: boolean;
	}

	let { label, description, value = $bindable(), disabled = false }: Props = $props();
</script>

<SettingsField {label} {description}>
	<Switch bind:checked={value} {disabled} />
</SettingsField>
