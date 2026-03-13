<!--
  Visual color harmony selector with wheel representation
  CSP-compliant: Uses SVG fill attributes (no inline styles)
-->
<script lang="ts">
	import { generateColorHarmony, type ColorHarmony } from 'src/lib/greater/utils';

	interface Props {
		seedColor: string;
		harmonyType?: keyof Omit<ColorHarmony, 'base'>;
		onSelect?: (colors: string[]) => void;
	}

	let { seedColor = $bindable(), harmonyType = 'complementary', onSelect }: Props = $props();

	let harmony = $derived(generateColorHarmony(seedColor));
	let selectedColors = $derived(harmony[harmonyType] || []);

	function handleColorClick(color: string) {
		if (onSelect) {
			onSelect([seedColor, color, ...selectedColors.filter((c) => c !== color)]);
		}
	}

	function handleSwatchKeyDown(event: KeyboardEvent, color: string) {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		handleColorClick(color);
	}
</script>

<div class="gr-color-harmony-picker">
	<div class="gr-color-harmony-picker__visualization">
		<button
			type="button"
			class="gr-color-harmony-picker__swatch gr-color-harmony-picker__swatch--base"
			onclick={() => handleColorClick(seedColor)}
			onkeydown={(event) => handleSwatchKeyDown(event, seedColor)}
			title="Base Color: {seedColor}"
			aria-label="Base Color: {seedColor}"
		>
			<svg class="gr-color-harmony-picker__swatch-svg" viewBox="0 0 60 60" aria-hidden="true">
				<circle
					class="gr-color-harmony-picker__swatch-circle"
					cx="30"
					cy="30"
					r="28"
					fill={seedColor}
				/>
			</svg>
			<span class="gr-color-harmony-picker__label">Base</span>
		</button>

		{#each selectedColors as color (color)}
			<button
				type="button"
				class="gr-color-harmony-picker__swatch gr-color-harmony-picker__swatch--harmony"
				onclick={() => handleColorClick(color)}
				onkeydown={(event) => handleSwatchKeyDown(event, color)}
				title="{harmonyType}: {color}"
				aria-label="{harmonyType}: {color}"
			>
				<svg class="gr-color-harmony-picker__swatch-svg" viewBox="0 0 60 60" aria-hidden="true">
					<circle
						class="gr-color-harmony-picker__swatch-circle"
						cx="30"
						cy="30"
						r="28"
						fill={color}
					/>
				</svg>
				<span class="gr-color-harmony-picker__label">{color}</span>
			</button>
		{/each}
	</div>

	<div class="gr-color-harmony-picker__info">
		<p>Harmony: <strong>{harmonyType}</strong></p>
	</div>
</div>

<style>
	.gr-color-harmony-picker {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
		background: var(--gr-color-surface);
		border-radius: var(--gr-radius-md);
		border: 1px solid var(--gr-color-border);
	}

	.gr-color-harmony-picker__visualization {
		display: flex;
		gap: 0.5rem;
		justify-content: center;
		align-items: center;
		flex-wrap: wrap;
	}

	.gr-color-harmony-picker__swatch {
		width: 60px;
		height: 60px;
		border-radius: 50%;
		border: 2px solid var(--gr-color-border);
		cursor: pointer;
		padding: 0;
		background: transparent;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: transform 0.2s;
		position: relative;
	}

	.gr-color-harmony-picker__swatch-svg {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}

	.gr-color-harmony-picker__swatch-circle {
		transform-origin: 50% 50%;
	}

	.gr-color-harmony-picker__swatch:hover {
		transform: scale(1.1);
	}

	.gr-color-harmony-picker__swatch--base {
		width: 80px;
		height: 80px;
		border-width: 3px;
		border-color: var(--gr-color-primary);
	}

	.gr-color-harmony-picker__label {
		font-size: 0.625rem;
		background: rgba(0, 0, 0, 0.5);
		color: white;
		padding: 2px 4px;
		border-radius: 4px;
		opacity: 0;
		transition: opacity 0.2s;
	}

	.gr-color-harmony-picker__swatch:hover .gr-color-harmony-picker__label {
		opacity: 1;
	}

	.gr-color-harmony-picker__info {
		text-align: center;
		font-size: 0.875rem;
		color: var(--gr-color-text-muted);
	}
</style>
