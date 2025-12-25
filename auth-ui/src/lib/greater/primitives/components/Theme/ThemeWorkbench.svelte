<!--
  Complete theme creation workbench
  Combines: color picker, harmony selector, contrast checker, preview
-->
<script lang="ts">
	import ColorHarmonyPicker from './ColorHarmonyPicker.svelte';
	import ContrastChecker from './ContrastChecker.svelte';
	import {
		generateTheme,
		type ThemeTokens,
		type ColorHarmony,
	} from 'src/lib/greater/utils';
	import Button from '../Button.svelte';
	import Card from '../Card.svelte';
	import SettingsSection from '../Settings/SettingsSection.svelte';
	import SettingsSelect from '../Settings/SettingsSelect.svelte';
	import ThemeProvider from '../ThemeProvider.svelte';
	import { untrack } from 'svelte';

	interface Props {
		initialColor?: string;
		onSave?: (theme: ThemeTokens) => void;
	}

	let { initialColor = '#3b82f6', onSave }: Props = $props();

	let seedColor = $state(untrack(() => initialColor));
	let harmonyType = $state<keyof Omit<ColorHarmony, 'base'>>('complementary');

	// Derived theme from seed color
	let theme = $derived(generateTheme(seedColor));

	const harmonyOptions = [
		{ value: 'complementary', label: 'Complementary' },
		{ value: 'analogous', label: 'Analogous' },
		{ value: 'triadic', label: 'Triadic' },
		{ value: 'tetradic', label: 'Tetradic' },
		{ value: 'splitComplementary', label: 'Split Complementary' },
		{ value: 'monochromatic', label: 'Monochromatic' },
	];

	function handleSave() {
		onSave?.(theme);
	}
</script>

<div class="theme-workbench">
	<div class="workbench-sidebar">
		<SettingsSection title="Theme Controls">
			<div class="color-input-group">
				<label for="seed-color">Primary Color</label>
				<div class="input-wrapper">
					<input type="color" id="seed-color" bind:value={seedColor} />
					<input type="text" bind:value={seedColor} class="hex-input" />
				</div>
			</div>

			<SettingsSelect label="Harmony" bind:value={harmonyType} options={harmonyOptions} />

			<div class="harmony-preview">
				<ColorHarmonyPicker {seedColor} {harmonyType} />
			</div>
		</SettingsSection>

		<SettingsSection title="Contrast Check">
			<ContrastChecker foreground={theme.colors.text} background={theme.colors.surface} />
			<div style="height: 1rem;"></div>
			<ContrastChecker foreground={theme.colors.primary[500]} background={theme.colors.surface} />
		</SettingsSection>

		<div class="actions">
			<Button variant="primary" onclick={handleSave} fullWidth>Save Theme</Button>
		</div>
	</div>

	<div class="workbench-preview">
		<ThemeProvider {theme}>
			<Card>
				<h2 style="margin-top: 0;">Component Preview</h2>
				<p>This is how your components will look with the generated theme.</p>

				<div class="component-grid">
					<Button variant="primary">Primary Button</Button>
					<Button variant="secondary">Secondary Button</Button>
					<Button variant="ghost">Ghost Button</Button>
				</div>

				<div class="swatch-grid">
					{#each Object.entries(theme.colors.primary) as [key, color] (key)}
						<div class="swatch" style="background-color: {color}" title="Primary {key}"></div>
					{/each}
				</div>
			</Card>
		</ThemeProvider>
	</div>
</div>

<style>
	.theme-workbench {
		display: grid;
		grid-template-columns: 350px 1fr;
		gap: 2rem;
		align-items: start;
	}

	@media (max-width: 768px) {
		.theme-workbench {
			grid-template-columns: 1fr;
		}
	}

	.color-input-group {
		margin-bottom: 1.5rem;
	}

	.color-input-group label {
		display: block;
		font-size: 0.875rem;
		font-weight: 500;
		margin-bottom: 0.5rem;
	}

	.input-wrapper {
		display: flex;
		gap: 0.5rem;
	}

	input[type='color'] {
		width: 40px;
		height: 40px;
		padding: 0;
		border: 1px solid var(--gr-color-border);
		border-radius: 4px;
		cursor: pointer;
	}

	.hex-input {
		flex: 1;
		padding: 0.5rem;
		border: 1px solid var(--gr-color-border);
		border-radius: 4px;
		font-family: monospace;
	}

	.actions {
		margin-top: 1rem;
	}

	.component-grid {
		display: flex;
		gap: 1rem;
		margin: 1.5rem 0;
		flex-wrap: wrap;
	}

	.swatch-grid {
		display: grid;
		grid-template-columns: repeat(10, 1fr);
		height: 40px;
		border-radius: 4px;
		overflow: hidden;
		margin-top: 1rem;
	}

	.swatch {
		height: 100%;
	}
</style>
