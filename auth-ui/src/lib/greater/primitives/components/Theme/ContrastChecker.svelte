<!--
  Live contrast ratio checker with WCAG compliance
-->
<script lang="ts">
	import { getContrastRatio, meetsWCAG } from 'src/lib/greater/utils';

	interface Props {
		foreground: string;
		background: string;
	}

	let { foreground, background }: Props = $props();

	let ratio = $derived(getContrastRatio(foreground, background));
	let passesAA = $derived(meetsWCAG(foreground, background, 'AA'));
	let passesAAA = $derived(meetsWCAG(foreground, background, 'AAA'));
	let passesAALarge = $derived(meetsWCAG(foreground, background, 'AA', 'large'));
</script>

<div class="contrast-checker">
	<div class="contrast-preview" style="color: {foreground}; background-color: {background}">
		<div class="preview-text">
			<h4 class="preview-heading">Large Text</h4>
			<p class="preview-body">Normal text sample</p>
		</div>
	</div>

	<div class="contrast-metrics">
		<div class="metric-row">
			<span class="metric-label">Contrast Ratio</span>
			<span class="metric-value {ratio < 3 ? 'fail' : ratio < 4.5 ? 'warn' : 'pass'}">
				{ratio.toFixed(2)}:1
			</span>
		</div>

		<div class="compliance-grid">
			<div class="compliance-item">
				<span class="compliance-label">AA Normal</span>
				<span class="compliance-badge {passesAA ? 'pass' : 'fail'}">
					{passesAA ? 'Pass' : 'Fail'}
				</span>
			</div>
			<div class="compliance-item">
				<span class="compliance-label">AA Large</span>
				<span class="compliance-badge {passesAALarge ? 'pass' : 'fail'}">
					{passesAALarge ? 'Pass' : 'Fail'}
				</span>
			</div>
			<div class="compliance-item">
				<span class="compliance-label">AAA Normal</span>
				<span class="compliance-badge {passesAAA ? 'pass' : 'fail'}">
					{passesAAA ? 'Pass' : 'Fail'}
				</span>
			</div>
		</div>
	</div>
</div>

<style>
	.contrast-checker {
		border: 1px solid var(--gr-color-border);
		border-radius: var(--gr-radius-md);
		overflow: hidden;
	}

	.contrast-preview {
		padding: 1.5rem;
		text-align: center;
		border-bottom: 1px solid var(--gr-color-border);
	}

	.preview-heading {
		margin: 0 0 0.5rem 0;
		font-size: 1.5rem;
		font-weight: bold;
	}

	.preview-body {
		margin: 0;
		font-size: 1rem;
	}

	.contrast-metrics {
		padding: 1rem;
		background: var(--gr-color-surface);
	}

	.metric-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.metric-value {
		font-weight: bold;
		font-family: monospace;
	}

	.compliance-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
	}

	.compliance-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
	}

	.compliance-label {
		font-size: 0.75rem;
		color: var(--gr-color-text-muted);
	}

	.compliance-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 2px 6px;
		border-radius: 4px;
	}

	.pass {
		color: var(--gr-color-success-text);
		background: var(--gr-color-success-bg);
	}
	.fail {
		color: var(--gr-color-error-text);
		background: var(--gr-color-error-bg);
	}
	.warn {
		color: var(--gr-color-warning-text);
		background: var(--gr-color-warning-bg);
	}
</style>
