<!--
  Visual color harmony selector with wheel representation
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
		// Optionally update seed color to clicked color?
		// seedColor = color;
	}
</script>

<div class="color-harmony-picker">
	<div class="color-wheel-visualization">
		<!-- Simplified visualization: just boxes for now -->
		<div
			class="color-swatch base"
			style="background-color: {seedColor}"
			onclick={() => handleColorClick(seedColor)}
			title="Base Color: {seedColor}"
			role="button"
			tabindex="0"
			onkeydown={(e) => e.key === 'Enter' && handleColorClick(seedColor)}
		>
			<span class="color-label">Base</span>
		</div>

		{#each selectedColors as color (color)}
			<div
				class="color-swatch harmony"
				style="background-color: {color}"
				onclick={() => handleColorClick(color)}
				title="{harmonyType}: {color}"
				role="button"
				tabindex="0"
				onkeydown={(e) => e.key === 'Enter' && handleColorClick(color)}
			>
				<span class="color-label">{color}</span>
			</div>
		{/each}
	</div>

	<div class="harmony-info">
		<p>Harmony: <strong>{harmonyType}</strong></p>
	</div>
</div>

<style>
	.color-harmony-picker {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
		background: var(--gr-color-surface);
		border-radius: var(--gr-radius-md);
		border: 1px solid var(--gr-color-border);
	}

	.color-wheel-visualization {
		display: flex;
		gap: 0.5rem;
		justify-content: center;
		align-items: center;
		flex-wrap: wrap;
	}

	.color-swatch {
		width: 60px;
		height: 60px;
		border-radius: 50%;
		border: 2px solid var(--gr-color-border);
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: transform 0.2s;
	}

	.color-swatch:hover {
		transform: scale(1.1);
	}

	.color-swatch.base {
		width: 80px;
		height: 80px;
		border-width: 3px;
		border-color: var(--gr-color-primary);
	}

	.color-label {
		font-size: 0.625rem;
		background: rgba(0, 0, 0, 0.5);
		color: white;
		padding: 2px 4px;
		border-radius: 4px;
		opacity: 0;
		transition: opacity 0.2s;
	}

	.color-swatch:hover .color-label {
		opacity: 1;
	}

	.harmony-info {
		text-align: center;
		font-size: 0.875rem;
		color: var(--gr-color-text-muted);
	}
</style>
