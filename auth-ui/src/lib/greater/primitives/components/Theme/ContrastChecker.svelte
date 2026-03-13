<!--
  Live contrast ratio checker with WCAG compliance
  CSP-compliant: Uses preset classes instead of inline styles
-->
<script lang="ts">
	import { getContrastRatio, meetsWCAG } from 'src/lib/greater/utils';

	type ContrastPreset = 'text-on-surface' | 'primary-on-surface' | 'text-on-primary' | 'custom';

	interface Props {
		/** Preset color combination to check */
		preset?: ContrastPreset;
		/** Custom foreground color (only used when preset='custom') */
		foreground?: string;
		/** Custom background color (only used when preset='custom') */
		background?: string;
	}

	let { preset = 'text-on-surface', foreground, background }: Props = $props();

	// Get colors from CSS custom properties based on preset
	// For custom preset, use provided colors
	// Note: Actual contrast calculation requires computed colors from CSS
	// This is a simplified version that shows the preset classes

	// Default colors for calculation (these would ideally be read from CSS)
	const presetColorsMap: Record<
		Exclude<ContrastPreset, 'custom'>,
		{ fg: string; bg: string; label: string }
	> = {
		'text-on-surface': { fg: '#1f2937', bg: '#ffffff', label: 'Text on Surface' },
		'primary-on-surface': { fg: '#3b82f6', bg: '#ffffff', label: 'Primary on Surface' },
		'text-on-primary': { fg: '#ffffff', bg: '#3b82f6', label: 'Text on Primary' },
	};

	let colors = $derived(
		preset === 'custom'
			? { fg: foreground || '#000000', bg: background || '#ffffff', label: 'Custom' }
			: presetColorsMap[preset]
	);

	let ratio = $derived(getContrastRatio(colors.fg, colors.bg));
	let passesAA = $derived(meetsWCAG(colors.fg, colors.bg, 'AA'));
	let passesAAA = $derived(meetsWCAG(colors.fg, colors.bg, 'AAA'));
	let passesAALarge = $derived(meetsWCAG(colors.fg, colors.bg, 'AA', 'large'));
</script>

<div class="gr-contrast-checker">
	<div class="gr-contrast-checker__preview gr-contrast-checker__preview--{preset}">
		<div class="gr-contrast-checker__preview-text">
			<h4 class="gr-contrast-checker__preview-heading">Large Text</h4>
			<p class="gr-contrast-checker__preview-body">Normal text sample</p>
		</div>
	</div>

	<div class="gr-contrast-checker__metrics">
		<div class="gr-contrast-checker__metric-row">
			<span class="gr-contrast-checker__metric-label">Contrast Ratio ({colors.label})</span>
			<span
				class="gr-contrast-checker__metric-value gr-contrast-checker__metric-value--{ratio < 3
					? 'fail'
					: ratio < 4.5
						? 'warn'
						: 'pass'}"
			>
				{ratio.toFixed(2)}:1
			</span>
		</div>

		<div class="gr-contrast-checker__compliance-grid">
			<div class="gr-contrast-checker__compliance-item">
				<span class="gr-contrast-checker__compliance-label">AA Normal</span>
				<span
					class="gr-contrast-checker__compliance-badge gr-contrast-checker__compliance-badge--{passesAA
						? 'pass'
						: 'fail'}"
				>
					{passesAA ? 'Pass' : 'Fail'}
				</span>
			</div>
			<div class="gr-contrast-checker__compliance-item">
				<span class="gr-contrast-checker__compliance-label">AA Large</span>
				<span
					class="gr-contrast-checker__compliance-badge gr-contrast-checker__compliance-badge--{passesAALarge
						? 'pass'
						: 'fail'}"
				>
					{passesAALarge ? 'Pass' : 'Fail'}
				</span>
			</div>
			<div class="gr-contrast-checker__compliance-item">
				<span class="gr-contrast-checker__compliance-label">AAA Normal</span>
				<span
					class="gr-contrast-checker__compliance-badge gr-contrast-checker__compliance-badge--{passesAAA
						? 'pass'
						: 'fail'}"
				>
					{passesAAA ? 'Pass' : 'Fail'}
				</span>
			</div>
		</div>
	</div>
</div>

<style>
	.gr-contrast-checker {
		border: 1px solid var(--gr-color-border);
		border-radius: var(--gr-radius-md);
		overflow: hidden;
	}

	.gr-contrast-checker__preview {
		padding: 1.5rem;
		text-align: center;
		border-bottom: 1px solid var(--gr-color-border);
	}

	/* Preset-based preview styles */
	.gr-contrast-checker__preview--text-on-surface {
		color: var(--gr-color-text, #1f2937);
		background-color: var(--gr-color-surface, #ffffff);
	}

	.gr-contrast-checker__preview--primary-on-surface {
		color: var(--gr-color-primary-500, #3b82f6);
		background-color: var(--gr-color-surface, #ffffff);
	}

	.gr-contrast-checker__preview--text-on-primary {
		color: var(--gr-color-primary-foreground, #ffffff);
		background-color: var(--gr-color-primary-500, #3b82f6);
	}

	.gr-contrast-checker__preview--custom {
		color: var(--gr-contrast-checker-fg, #000000);
		background-color: var(--gr-contrast-checker-bg, #ffffff);
	}

	.gr-contrast-checker__preview-heading {
		margin: 0 0 0.5rem 0;
		font-size: 1.5rem;
		font-weight: bold;
	}

	.gr-contrast-checker__preview-body {
		margin: 0;
		font-size: 1rem;
	}

	.gr-contrast-checker__metrics {
		padding: 1rem;
		background: var(--gr-color-surface);
	}

	.gr-contrast-checker__metric-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.gr-contrast-checker__metric-value {
		font-weight: bold;
		font-family: monospace;
	}

	.gr-contrast-checker__compliance-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
	}

	.gr-contrast-checker__compliance-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
	}

	.gr-contrast-checker__compliance-label {
		font-size: 0.75rem;
		color: var(--gr-color-text-muted);
	}

	.gr-contrast-checker__compliance-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 2px 6px;
		border-radius: 4px;
	}

	.gr-contrast-checker__metric-value--pass,
	.gr-contrast-checker__compliance-badge--pass {
		color: var(--gr-color-success-text);
		background: var(--gr-color-success-bg);
	}

	.gr-contrast-checker__metric-value--fail,
	.gr-contrast-checker__compliance-badge--fail {
		color: var(--gr-color-error-text);
		background: var(--gr-color-error-bg);
	}

	.gr-contrast-checker__metric-value--warn {
		color: var(--gr-color-warning-text);
		background: var(--gr-color-warning-bg);
	}
</style>
