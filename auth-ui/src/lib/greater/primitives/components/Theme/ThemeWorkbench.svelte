<!--
  Complete theme creation workbench
  Combines: color picker, harmony selector, contrast checker, preview
  CSP-compliant: Uses CSS custom properties instead of inline styles
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

	// Primary color scale keys for swatch grid
	const primaryScaleKeys = [
		'50',
		'100',
		'200',
		'300',
		'400',
		'500',
		'600',
		'700',
		'800',
		'900',
	] as const;

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

<div class="gr-theme-workbench">
	<div class="gr-theme-workbench__sidebar">
		<SettingsSection title="Theme Controls">
			<div class="gr-theme-workbench__color-input-group">
				<label for="seed-color">Primary Color</label>
				<div class="gr-theme-workbench__input-wrapper">
					<input type="color" id="seed-color" bind:value={seedColor} />
					<input type="text" bind:value={seedColor} class="gr-theme-workbench__hex-input" />
				</div>
			</div>

			<SettingsSelect label="Harmony" bind:value={harmonyType} options={harmonyOptions} />

			<div class="gr-theme-workbench__harmony-preview">
				<ColorHarmonyPicker {seedColor} {harmonyType} />
			</div>
		</SettingsSection>

		<SettingsSection title="Contrast Check">
			<ContrastChecker preset="text-on-surface" />
			<div class="gr-theme-workbench__spacer"></div>
			<ContrastChecker preset="primary-on-surface" />
		</SettingsSection>

		<div class="gr-theme-workbench__actions">
			<Button variant="solid" onclick={handleSave}>Save Theme</Button>
		</div>
	</div>

	<div class="gr-theme-workbench__preview">
		<ThemeProvider>
			<Card>
				<h2 class="gr-theme-workbench__preview-heading">Component Preview</h2>
				<p>This is how your components will look with the generated theme.</p>

				<div class="gr-theme-workbench__component-grid">
					<Button variant="solid">Primary Button</Button>
					<Button variant="outline">Secondary Button</Button>
					<Button variant="ghost">Ghost Button</Button>
				</div>

				<div class="gr-theme-workbench__swatch-grid">
					{#each primaryScaleKeys as key (key)}
						<div class="gr-swatch gr-swatch--primary-{key}" title="Primary {key}"></div>
					{/each}
				</div>
			</Card>
		</ThemeProvider>
	</div>
</div>

<style>
	.gr-theme-workbench {
		display: grid;
		grid-template-columns: 350px 1fr;
		gap: 2rem;
		align-items: start;
	}

	@media (max-width: 768px) {
		.gr-theme-workbench {
			grid-template-columns: 1fr;
		}
	}

	.gr-theme-workbench__color-input-group {
		margin-bottom: 1.5rem;
	}

	.gr-theme-workbench__color-input-group label {
		display: block;
		font-size: 0.875rem;
		font-weight: 500;
		margin-bottom: 0.5rem;
	}

	.gr-theme-workbench__input-wrapper {
		display: flex;
		gap: 0.5rem;
	}

	.gr-theme-workbench__input-wrapper input[type='color'] {
		width: 40px;
		height: 40px;
		padding: 0;
		border: 1px solid var(--gr-color-border);
		border-radius: 4px;
		cursor: pointer;
	}

	.gr-theme-workbench__hex-input {
		flex: 1;
		padding: 0.5rem;
		border: 1px solid var(--gr-color-border);
		border-radius: 4px;
		font-family: monospace;
	}

	.gr-theme-workbench__actions {
		margin-top: 1rem;
	}

	.gr-theme-workbench__spacer {
		height: 1rem;
	}

	.gr-theme-workbench__preview-heading {
		margin-top: 0;
	}

	.gr-theme-workbench__component-grid {
		display: flex;
		gap: 1rem;
		margin: 1.5rem 0;
		flex-wrap: wrap;
	}

	.gr-theme-workbench__swatch-grid {
		display: grid;
		grid-template-columns: repeat(10, 1fr);
		height: 40px;
		border-radius: 4px;
		overflow: hidden;
		margin-top: 1rem;
	}
</style>
