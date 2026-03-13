<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	export type WidthPreset = 'full' | '1/2' | '1/3' | '2/3' | '1/4' | '3/4' | 'content' | 'auto';
	export type HeightPreset = 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';

	interface Props extends HTMLAttributes<HTMLElement> {
		variant?: 'text' | 'circular' | 'rectangular' | 'rounded';
		width?: WidthPreset;
		height?: HeightPreset;
		animation?: 'pulse' | 'wave' | 'none';
		class?: string;
		loading?: boolean;
		children?: Snippet;
	}

	let {
		variant = 'text',
		width,
		height,
		animation = 'pulse',
		class: className = '',
		loading = true,
		children,
		id,
		onclick,
		onmouseenter,
		onmouseleave,
		onfocus,
		onblur,
		onkeydown,
		onkeyup,
		role,
		'aria-label': ariaLabel,
		'aria-labelledby': ariaLabelledby,
		'aria-describedby': ariaDescribedby,
		tabindex,
	}: Props = $props();

	const INTERACTIVE_ROLES = new Set([
		'button',
		'checkbox',
		'combobox',
		'link',
		'menuitem',
		'menuitemcheckbox',
		'menuitemradio',
		'option',
		'radio',
		'searchbox',
		'slider',
		'spinbutton',
		'switch',
		'tab',
		'textbox',
		'treeitem',
	]);

	const parsedTabIndex = $derived.by<number | undefined>(() => {
		if (tabindex === undefined || tabindex === null) {
			return undefined;
		}
		if (typeof tabindex === 'number') {
			return Number.isFinite(tabindex) ? tabindex : undefined;
		}
		const numericValue = Number(tabindex);
		return Number.isFinite(numericValue) ? numericValue : undefined;
	});

	const hasInteractiveHandlers = $derived(Boolean(onclick || onkeydown || onkeyup));

	const isInteractiveRole = (roleValue: string | null | undefined): boolean => {
		if (!roleValue) {
			return false;
		}
		return INTERACTIVE_ROLES.has(roleValue);
	};

	const isInteractive = $derived.by(() => {
		if (isInteractiveRole(role)) {
			return true;
		}
		if (hasInteractiveHandlers) {
			return true;
		}
		const parsedValue = parsedTabIndex;
		if (parsedValue !== undefined && parsedValue >= 0) {
			return true;
		}
		return false;
	});

	// Compute skeleton classes
	const skeletonClass = $derived.by(() => {
		const classes = [
			'gr-skeleton',
			`gr-skeleton--${variant}`,
			animation !== 'none' && `gr-skeleton--${animation}`,
			width && `gr-skeleton--width-${width}`,
			height && `gr-skeleton--height-${height}`,
			className,
		]
			.filter(Boolean)
			.join(' ');

		return classes;
	});
</script>

{#snippet SkeletonContent()}
	{#if animation === 'wave'}
		<div class="gr-skeleton__wave"></div>
	{/if}
{/snippet}

{#if loading}
	{#if isInteractive}
		<button
			class={skeletonClass}
			{role}
			aria-label={ariaLabel ?? 'Loading'}
			aria-labelledby={ariaLabelledby}
			aria-describedby={ariaDescribedby}
			{id}
			{onclick}
			{onmouseenter}
			{onmouseleave}
			{onfocus}
			{onblur}
			{onkeydown}
			{onkeyup}
			tabindex={parsedTabIndex}
			type="button"
		>
			{@render SkeletonContent()}
		</button>
	{:else}
		<div
			class={skeletonClass}
			aria-hidden="true"
			role={role ?? 'status'}
			aria-label={ariaLabel ?? 'Loading'}
			aria-labelledby={ariaLabelledby}
			aria-describedby={ariaDescribedby}
			{id}
			{onclick}
			{onmouseenter}
			{onmouseleave}
			{onfocus}
			{onblur}
			{onkeydown}
			{onkeyup}
		>
			{@render SkeletonContent()}
		</div>
	{/if}
{:else if children}
	{@render children()}
{/if}
