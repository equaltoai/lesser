import type { HTMLAttributes } from 'svelte/elements';
import type { Snippet } from 'svelte';
export interface DroppedItem {
	id: string;
	type: 'file' | 'text' | 'url';
	content: string;
	file?: File;
	name?: string;
	size?: number;
	mimeType?: string;
}
export interface DropError {
	code: 'FILE_TOO_LARGE' | 'TOO_MANY_FILES' | 'INVALID_TYPE' | 'READ_ERROR';
	message: string;
	file?: File;
}
interface Props extends HTMLAttributes<HTMLDivElement> {
	/**
	 * Accepted content types.
	 */
	accept?: {
		files?: string[];
		text?: boolean;
		urls?: boolean;
	};
	/**
	 * Maximum number of files allowed.
	 * @default undefined (unlimited)
	 */
	maxFiles?: number;
	/**
	 * Maximum file size in bytes.
	 * @default 10485760 (10MB)
	 */
	maxSize?: number;
	/**
	 * Allow multiple files.
	 * @default true
	 */
	multiple?: boolean;
	/**
	 * Visual variant.
	 * @default 'outlined'
	 */
	variant?: 'outlined' | 'filled' | 'minimal';
	/**
	 * Disabled state.
	 */
	disabled?: boolean;
	/**
	 * Additional CSS classes.
	 */
	class?: string;
	/**
	 * Callback when items are dropped.
	 */
	onDrop?: (items: DroppedItem[]) => void;
	/**
	 * Callback on error.
	 */
	onError?: (error: DropError) => void;
	/**
	 * Callback on drag enter.
	 */
	onDragEnter?: () => void;
	/**
	 * Callback on drag leave.
	 */
	onDragLeave?: () => void;
	/**
	 * Content snippet.
	 */
	children?: Snippet;
}
/**
 * DropZone component - Drag and drop file upload area with validation and mobile fallback.
 *
 *
 * @example
 * ```svelte
 * <DropZone accept={{ files: ['.png', '.jpg'], text: true }} onDrop={handleDrop}>
 * <p>Drop files here</p>
 * </DropZone>
 * ```
 */
declare const DropZone: import('svelte').Component<Props, {}, ''>;
type DropZone = ReturnType<typeof DropZone>;
export default DropZone;
//# sourceMappingURL=DropZone.svelte.d.ts.map
