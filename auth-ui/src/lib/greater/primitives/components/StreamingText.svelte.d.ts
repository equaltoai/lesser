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
/**
 * StreamingText component - Displays text with a typewriter/streaming cursor effect.
 * Designed to work with externally streaming content (e.g. AI responses).
 *
 *
 * @example
 * ```svelte
 * <StreamingText content={streamedText} streaming={isStreaming} onComplete={() => console.log('Done')} />
 * ```
 */
declare const StreamingText: import('svelte').Component<Props, {}, ''>;
type StreamingText = ReturnType<typeof StreamingText>;
export default StreamingText;
//# sourceMappingURL=StreamingText.svelte.d.ts.map
