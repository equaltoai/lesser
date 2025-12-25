<!--
DropZone component - Drag and drop file upload area with validation and mobile fallback.

@component
@example
```svelte
<DropZone accept={{ files: ['.png', '.jpg'], text: true }} onDrop={handleDrop}>
  <p>Drop files here</p>
</DropZone>
```
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	export interface DroppedItem {
		id: string;
		type: 'file' | 'text' | 'url';
		content: string; // Text content or data URL or empty if file not read yet (wrapper mode often handles reading separately, but here we might read it)
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

	interface Props {
		/**
		 * Accepted content types.
		 */
		accept?: {
			files?: string[]; // MIME types or extensions (e.g. ['.png', 'image/*'])
			text?: boolean; // Accept dropped text
			urls?: boolean; // Accept dropped URLs
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

	let {
		accept = {},
		maxFiles,
		maxSize = 10485760, // 10MB
		multiple = true,
		variant = 'outlined',
		disabled = false,
		class: className = '',
		onDrop,
		onError,
		onDragEnter,
		onDragLeave,
		children,
		...restProps
	}: Props = $props();

	let isOver = $state(false);
	let fileInput: HTMLInputElement;
	let droppedItemIdCounter = 0;

	function nextDroppedItemId(type: DroppedItem['type']): string {
		droppedItemIdCounter += 1;
		return `dropzone-${type}-${Date.now()}-${droppedItemIdCounter}`;
	}

	function handleDragEnter(e: DragEvent) {
		if (disabled) return;
		e.preventDefault();
		e.stopPropagation();
		isOver = true;
		onDragEnter?.();
	}

	function handleDragOver(e: DragEvent) {
		if (disabled) return;
		e.preventDefault();
		e.stopPropagation();
		// Essential to allow drop
	}

	function handleDragLeave(e: DragEvent) {
		if (disabled) return;
		e.preventDefault();
		e.stopPropagation();
		// Ensure we are actually leaving the dropzone, not just entering a child
		if (e.currentTarget === e.target) {
			isOver = false;
			// We don't set isDragging to false immediately to avoid flicker if moving between children?
			// Actually, standard behavior is isDragging tracks if drag operation is active generally, but for DropZone visual state 'isOver' is key.
			// If we leave the dropzone, visual feedback should stop.
			onDragLeave?.();
		}
		// Simple implementation: toggle state
		isOver = false;
	}

	// Better drag leave handling to avoid flicker when dragging over children
	// We can use a counter or check relatedTarget, but for Svelte simpler logic usually works if pointer-events: none on children during drag?
	// Or just rely on the fact that 'dragleave' fires on the parent when entering a child.
	// A common robust pattern is checking `e.relatedTarget`.
	// However, `e.currentTarget === e.target` check in handleDragLeave handles the "leaving to outside" case mostly.

	async function handleDrop(e: DragEvent) {
		if (disabled) return;
		e.preventDefault();
		e.stopPropagation();
		isOver = false;

		const dataTransfer = e.dataTransfer;
		if (!dataTransfer) return;

		const droppedItems: DroppedItem[] = [];

		// Handle Files
		if (dataTransfer.files && dataTransfer.files.length > 0) {
			// Validate maxFiles
			if (maxFiles && dataTransfer.files.length > maxFiles) {
				onError?.({
					code: 'TOO_MANY_FILES',
					message: `Too many files dropped. Maximum is ${maxFiles}.`,
				});
				return;
			}

			if (!multiple && dataTransfer.files.length > 1) {
				onError?.({
					code: 'TOO_MANY_FILES',
					message: `Multiple files not allowed.`,
				});
				return;
			}

			const files = Array.from(dataTransfer.files);
			for (const file of files) {
				const error = validateFile(file);
				if (error) {
					onError?.(error);
					// Continue or stop? Usually stop entire batch or skip invalid.
					// Let's skip invalid but report error.
					continue;
				}

				// Read file content?
				// The spec implies we return DroppedItem with content.
				// For large files, reading automatically might be bad.
				// But "DropZone - Drag-and-drop files/text (needed for chat context)" usually implies reading text content.
				// Let's try to read as text if text/plain or similar, or dataURL for images?
				// Or maybe just return the File object and let consumer read it, but DroppedItem has 'content' string.
				// The spec says: "content: string; // Text content or data URL"

				try {
					let content = '';
					if (
						file.type.startsWith('text/') ||
						file.name.endsWith('.json') ||
						file.name.endsWith('.md') ||
						file.name.endsWith('.ts') ||
						file.name.endsWith('.js')
					) {
						content = await readFileAsText(file);
					} else {
						// For binary/images, read as DataURL
						content = await readFileAsDataURL(file);
					}

					droppedItems.push({
						id: nextDroppedItemId('file'),
						type: 'file',
						content,
						file,
						name: file.name,
						size: file.size,
						mimeType: file.type,
					});
				} catch {
					onError?.({
						code: 'READ_ERROR',
						message: `Failed to read file ${file.name}`,
						file,
					});
				}
			}
		}

		// Handle Text/URL if enabled and no files dropped (or mixed?)
		// Usually drag operation is either files or text.
		if (droppedItems.length === 0) {
			const text = dataTransfer.getData('text/plain');
			const uriList = dataTransfer.getData('text/uri-list');

			if (accept.urls && (uriList || (text && isValidUrl(text)))) {
				const url = uriList || text;
				droppedItems.push({
					id: nextDroppedItemId('url'),
					type: 'url',
					content: url,
					name: url,
				});
			} else if (accept.text && text) {
				droppedItems.push({
					id: nextDroppedItemId('text'),
					type: 'text',
					content: text,
					name: text.slice(0, 20) + (text.length > 20 ? '...' : ''),
				});
			}
		}

		if (droppedItems.length > 0) {
			onDrop?.(droppedItems);
		}
	}

	function validateFile(file: File): DropError | null {
		if (maxSize && file.size > maxSize) {
			return {
				code: 'FILE_TOO_LARGE',
				message: `File ${file.name} is too large. Max size is ${formatBytes(maxSize)}.`,
				file,
			};
		}

		if (accept.files && accept.files.length > 0) {
			const allowed = accept.files.some((type) => {
				if (type.startsWith('.')) {
					return file.name.toLowerCase().endsWith(type.toLowerCase());
				}
				// wildcard mime type
				if (type.endsWith('/*')) {
					const baseType = type.slice(0, -2);
					return file.type.startsWith(baseType);
				}
				return file.type === type;
			});

			if (!allowed) {
				return {
					code: 'INVALID_TYPE',
					message: `File type ${file.type} not supported.`,
					file,
				};
			}
		}

		return null;
	}

	async function handleFileInputChange(e: Event) {
		const target = e.target as HTMLInputElement;
		if (target.files && target.files.length > 0) {
			// Reuse drop logic or extract
			const files = Array.from(target.files);
			const droppedItems: DroppedItem[] = [];

			for (const file of files) {
				const error = validateFile(file);
				if (error) {
					onError?.(error);
					continue;
				}

				try {
					let content = '';
					// Simple heuristics for reading
					if (
						file.type.startsWith('text/') ||
						file.name.match(/\.(json|md|txt|csv|xml|svg|html|css|js|ts|svelte)$/i)
					) {
						content = await readFileAsText(file);
					} else {
						content = await readFileAsDataURL(file);
					}

					droppedItems.push({
						id: nextDroppedItemId('file'),
						type: 'file',
						content,
						file,
						name: file.name,
						size: file.size,
						mimeType: file.type,
					});
				} catch {
					onError?.({
						code: 'READ_ERROR',
						message: `Failed to read file ${file.name}`,
						file,
					});
				}
			}

			if (droppedItems.length > 0) {
				onDrop?.(droppedItems);
			}

			// Reset input value to allow selecting same file again
			target.value = '';
		}
	}

	function triggerFileInput() {
		if (!disabled && fileInput) {
			fileInput.click();
		}
	}

	// Helpers
	function readFileAsText(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(reader.result as string);
			reader.onerror = () => reject(reader.error);
			reader.readAsText(file);
		});
	}

	function readFileAsDataURL(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(reader.result as string);
			reader.onerror = () => reject(reader.error);
			reader.readAsDataURL(file);
		});
	}

	function isValidUrl(string: string) {
		try {
			new URL(string);
			return true;
		} catch {
			return false;
		}
	}

	function formatBytes(bytes: number, decimals = 2) {
		if (bytes === 0) return '0 Bytes';
		const k = 1024;
		const dm = decimals < 0 ? 0 : decimals;
		const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
	}

	// Construct accept string for input - use $derived to track accept prop changes
	const acceptString = $derived(accept.files?.join(',') || '');
</script>

<div
	class="gr-drop-zone gr-drop-zone--{variant} {className}"
	class:gr-drop-zone--active={isOver}
	class:gr-drop-zone--disabled={disabled}
	ondragenter={handleDragEnter}
	ondragover={handleDragOver}
	ondragleave={handleDragLeave}
	ondrop={handleDrop}
	onclick={triggerFileInput}
	onkeydown={(e) => {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			triggerFileInput();
		}
	}}
	role="button"
	tabindex={disabled ? -1 : 0}
	aria-disabled={disabled}
	{...restProps}
>
	<input
		bind:this={fileInput}
		type="file"
		accept={acceptString}
		{multiple}
		onchange={handleFileInputChange}
		onclick={(e) => e.stopPropagation()}
		class="gr-file-input"
		tabindex="-1"
		aria-hidden="true"
	/>
	{@render children?.()}
</div>
