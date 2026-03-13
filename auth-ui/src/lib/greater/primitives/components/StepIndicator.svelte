<!--
StepIndicator component - Numbered badge for tutorials and multi-step workflows.

@component
@example
```svelte
<StepIndicator number={1} state="active" label="Step 1" />
<StepIndicator number={2} state="pending" />
<StepIndicator number={3} state="completed" />
```
-->
<script lang="ts">
	import { CheckIcon, XIcon } from 'src/lib/greater/icons';
	import type { Component } from 'svelte';

	interface Props {
		/**
		 * Step number or character.
		 */
		number: number | string;

		/**
		 * Optional label displayed below the indicator.
		 */
		label?: string;

		/**
		 * Visual style variant.
		 */
		variant?: 'filled' | 'outlined' | 'ghost';

		/**
		 * Size of the indicator.
		 */
		size?: 'sm' | 'md' | 'lg';

		/**
		 * Current state of the step.
		 */
		state?: 'pending' | 'active' | 'completed' | 'error';

		/**
		 * Color theme (overridden by state in some cases).
		 */
		color?: 'primary' | 'success' | 'warning' | 'error';

		/**
		 * Custom icon to display instead of number.
		 */
		icon?: Component;

		/**
		 * Additional CSS classes.
		 */
		class?: string;
	}

	let {
		number,
		label,
		variant = 'filled',
		size = 'md',
		state = 'active',
		color = 'primary',
		icon,
		class: className = '',
		style: _style,
		...restProps
	}: Props & { style?: string } = $props();

	// Icons based on state
	const displayIcon = $derived.by(() => {
		if (icon) return icon;
		if (state === 'completed') return CheckIcon;
		if (state === 'error') return XIcon;
		return null;
	});

	// Color based on state
	const colorClass = $derived.by(() => {
		if (state === 'pending') return 'gray';
		if (state === 'completed') return 'success';
		if (state === 'error') return 'error';
		return color;
	});

	const badgeClass = $derived(
		[
			'gr-step-badge',
			`gr-step-badge--${variant}`,
			`gr-step-badge--${size}`,
			`gr-step-badge--${colorClass}`,
		]
			.filter(Boolean)
			.join(' ')
	);

	const iconSizeMap = { sm: 16, md: 20, lg: 24 };
	const iconSize = $derived(iconSizeMap[size]);
</script>

<div class="gr-step-indicator {className}" {...restProps}>
	<div class={badgeClass} role="img" aria-label={`Step ${number}${label ? ': ' + label : ''}`}>
		{#if displayIcon}
			{@const Icon = displayIcon}
			<Icon size={iconSize} />
		{:else}
			<span class="gr-step-number">{number}</span>
		{/if}
	</div>

	{#if label}
		<span class="gr-step-label">{label}</span>
	{/if}
</div>
