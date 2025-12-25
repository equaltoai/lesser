<!--
IconBadge component - A container for icons with consistent shapes, sizes, and colors.

@component
@example
```svelte
<IconBadge icon={TargetIcon} size="lg" color="primary" />
<IconBadge icon={SettingsIcon} variant="outlined" shape="square" />
```
-->
<script lang="ts">
	import type { Component, Snippet } from 'svelte';

	interface Props {
		/**
		 * Icon component to display.
		 */
		icon?: Component;

		/**
		 * Override icon size. Defaults based on badge size.
		 */
		iconSize?: number;

		/**
		 * Size of the badge.
		 */
		size?: 'sm' | 'md' | 'lg' | 'xl';

		/**
		 * Color theme.
		 */
		color?: 'primary' | 'success' | 'warning' | 'error' | 'gray';

		/**
		 * Visual variant.
		 */
		variant?: 'filled' | 'outlined' | 'ghost';

		/**
		 * Shape of the badge.
		 */
		shape?: 'circle' | 'rounded' | 'square';

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Snippet for icon content (alternative to icon prop).
		 */
		children?: Snippet;
	}

	let {
		icon,
		iconSize,
		size = 'md',
		color = 'primary',
		variant = 'filled',
		shape = 'circle',
		class: className = '',
		children,
		...restProps
	}: Props = $props();

	const sizeMap = { sm: 40, md: 56, lg: 72, xl: 96 };
	const defaultIconSizeMap = { sm: 20, md: 28, lg: 36, xl: 48 };

	const badgeSize = $derived(sizeMap[size]);
	const finalIconSize = $derived(iconSize || defaultIconSizeMap[size]);

	const badgeClass = $derived(
		[
			'gr-icon-badge',
			`gr-icon-badge--${variant}`,
			`gr-icon-badge--${color}`,
			`gr-icon-badge--${shape}`,
			className,
		]
			.filter(Boolean)
			.join(' ')
	);
</script>

<div class={badgeClass} style:width="{badgeSize}px" style:height="{badgeSize}px" {...restProps}>
	{#if icon}
		{@const Icon = icon}
		<Icon size={finalIconSize} />
	{:else if children}
		{@render children()}
	{/if}
</div>
