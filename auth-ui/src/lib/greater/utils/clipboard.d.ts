/**
 * Clipboard utilities for Greater Components
 * Provides cross-browser copy functionality with fallbacks.
 * @module clipboard
 */
/**
 * Result of a copy operation
 */
export interface CopyResult {
	/** Whether the copy operation was successful */
	success: boolean;
	/** Error message if the operation failed */
	error?: string;
}
/**
 * Copies text to the clipboard with fallback for older browsers or non-secure contexts.
 *
 * @param text - The text to copy to the clipboard
 * @returns Promise resolving to the result of the operation
 */
export declare function copyToClipboard(text: string): Promise<CopyResult>;
/**
 * Copies the text content of a specific DOM element.
 *
 * @param element - The HTMLElement to copy text from
 * @returns Promise resolving to the result of the operation
 */
export declare function copyElementText(element: HTMLElement): Promise<CopyResult>;
/**
 * Copies code from a code block, stripping any extra HTML artifacts if necessary.
 * Ideal for <pre><code>...</code></pre> structures.
 *
 * @param codeElement - The HTMLElement containing the code
 * @returns Promise resolving to the result of the operation
 */
export declare function copyCodeBlock(codeElement: HTMLElement): Promise<CopyResult>;
//# sourceMappingURL=clipboard.d.ts.map
