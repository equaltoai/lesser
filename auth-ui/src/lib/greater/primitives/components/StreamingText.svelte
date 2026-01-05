<!--
StreamingText component - Displays text with a typewriter/streaming cursor effect.
Designed to work with externally streaming content (e.g. AI responses).

@component
@example
```svelte
<StreamingText content={streamedText} streaming={isStreaming} onComplete={() => console.log('Done')} />
```
-->
<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		/**
		 * The text content to display.
		 */
		content: string;

		/**
		 * Whether the content is currently streaming (controls cursor).
		 * @default true
		 */
		streaming?: boolean;

		/**
		 * Show the blinking cursor.
		 * @default true
		 */
		showCursor?: boolean;

		/**
		 * Character to use for the cursor.
		 * @default '▊'
		 */
		cursorChar?: string;

		/**
		 * Speed is not used for animation here as we rely on external updates,
		 * but kept for API compatibility or future smoothing.
		 */
		speed?: 'slow' | 'natural' | 'fast' | number;

		/**
		 * Element tag to render.
		 */
		as?: string;

		/**
		 * Additional CSS classes.
		 */
		class?: string;

		/**
		 * Callback when streaming finishes (streaming prop becomes false).
		 */
		onComplete?: () => void;
	}

	let {
		content,
		streaming = true,
		showCursor = true,
		cursorChar = '▊',
		// speed = 'natural', // Unused
		as: Tag = 'span',
		class: className = '',
		onComplete,
		...restProps
	}: Props = $props();

	let displayedContent = $state('');
	let cursorVisible = $state(true);
	let cursorInterval: ReturnType<typeof setInterval>;

	// Sync content
	$effect(() => {
		displayedContent = content;
	});

	// Handle completion
	$effect(() => {
		if (!streaming && onComplete) {
			// We use a timeout to ensure this runs after the last content update
			setTimeout(() => {
				onComplete();
			}, 0);
		}
	});

	onMount(() => {
		cursorInterval = setInterval(() => {
			if (showCursor && streaming) {
				cursorVisible = !cursorVisible;
			} else {
				cursorVisible = false;
			}
		}, 500);
	});

	onDestroy(() => {
		if (cursorInterval) clearInterval(cursorInterval);
	});
</script>

<svelte:element this={Tag} class="gr-streaming-text {className}" {...restProps}>
	{displayedContent}
	{#if streaming && showCursor}
		<span class="gr-cursor" style:opacity={cursorVisible ? 1 : 0}>{cursorChar}</span>
	{/if}
</svelte:element>
