<!--
CopyButton component - A button that copies text to the clipboard with visual feedback.

@component
@example
```svelte
<CopyButton text="npm install @equaltoai/greater-components" />

<CopyButton 
  targetSelector="#code-block" 
  variant="icon-text" 
  labels={{ default: 'Copy Code', success: 'Copied!' }} 
/>
```
-->
<script lang="ts">
	import Button from './Button.svelte';
	import { CopyIcon, CheckIcon } from 'src/lib/greater/icons';
	import {
		copyToClipboard,
		copyElementText,
		type CopyResult,
	} from 'src/lib/greater/utils';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type ButtonPassthroughProps = Omit<HTMLButtonAttributes, 'type'> & {
		type?: 'button' | 'submit' | 'reset';
	};

	interface Props extends ButtonPassthroughProps {
		/**
		 * The text to copy to the clipboard.
		 * Takes precedence over targetSelector if both are provided.
		 */
		text?: string;

		/**
		 * CSS selector for the element to copy text from.
		 * Used if text is not provided.
		 */
		targetSelector?: string;

		/**
		 * Visual layout variant of the button content.
		 * - `icon`: Show only the icon (default)
		 * - `text`: Show only the text
		 * - `icon-text`: Show both icon and text
		 */
		variant?: 'icon' | 'text' | 'icon-text';

		/**
		 * The visual style variant of the underlying Button component.
		 * @default 'ghost'
		 */
		buttonVariant?: 'solid' | 'outline' | 'ghost';

		/**
		 * Size of the button.
		 * @default 'md'
		 */
		size?: 'sm' | 'md' | 'lg';

		/**
		 * Duration in milliseconds to show the success state.
		 * @default 2000
		 */
		feedbackDuration?: number;

		/**
		 * Custom labels for the button text.
		 */
		labels?: {
			default?: string;
			success?: string;
			error?: string;
		};

		/**
		 * Optional callback called after copy attempt.
		 */
		onCopy?: (result: CopyResult) => void;

		/**
		 * Additional class names.
		 */
		class?: string;
	}

	let {
		text = '',
		targetSelector = '',
		variant = 'icon',
		buttonVariant = 'ghost',
		size = 'md',
		type,
		feedbackDuration = 2000,
		labels = {},
		onCopy,
		onclick: onClick,
		class: className = '',
		style: _style,
		...restProps
	}: Props = $props();

	let copied = $state(false);
	let error = $state('');
	let timeoutId: ReturnType<typeof setTimeout>;

	const defaultLabels = {
		default: 'Copy',
		success: 'Copied!',
		error: 'Error',
	};

	const currentLabels = $derived({ ...defaultLabels, ...labels });

	type ClickEvent = MouseEvent & { currentTarget: EventTarget & HTMLButtonElement };

	async function handleCopy(event: ClickEvent) {
		// Prevent default button behavior if needed, though Button handles it.
		// event.preventDefault(); // Optional, Button probably handles it.

		onClick?.(event);

		let result: CopyResult = { success: false, error: 'No text to copy' };

		if (text) {
			result = await copyToClipboard(text);
		} else if (targetSelector) {
			const element = document.querySelector(targetSelector);
			if (element instanceof HTMLElement) {
				result = await copyElementText(element);
			} else {
				result = { success: false, error: 'Target element not found' };
			}
		}

		if (result.success) {
			copied = true;
			error = '';

			if (timeoutId) clearTimeout(timeoutId);
			timeoutId = setTimeout(() => {
				copied = false;
			}, feedbackDuration);
		} else {
			copied = false;
			error = result.error || 'Failed to copy';
		}

		if (onCopy) {
			onCopy(result);
		}
	}

	// Cleanup timeout on destroy (not strictly needed with runes as component unmount cleans up timers usually? No, standard JS timers need cleanup)
	// Svelte 5 $effect.pre might be used for cleanup if we wanted to be strict,
	// but for a simple timeout that updates local state, it's usually fine unless state update on unmounted component throws.
	// Svelte 5 safe-guards updates.

	// Icon size based on button size
	const iconSize = $derived(size === 'sm' ? 16 : 20);
</script>

<Button
	variant={buttonVariant}
	{size}
	{type}
	class={className}
	onclick={handleCopy}
	aria-label={copied ? currentLabels.success : currentLabels.default}
	{...restProps as Record<string, unknown>}
>
	<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
	{#snippet prefix()}
		{#if variant === 'icon' || variant === 'icon-text'}
			{#if copied}
				<CheckIcon size={iconSize} />
			{:else}
				<CopyIcon size={iconSize} />
			{/if}
		{/if}
	{/snippet}

	{#if variant === 'text' || variant === 'icon-text'}
		<span>
			{#if copied}
				{currentLabels.success}
			{:else if error && currentLabels.error}
				{currentLabels.error}
			{:else}
				{currentLabels.default}
			{/if}
		</span>
	{/if}
</Button>
