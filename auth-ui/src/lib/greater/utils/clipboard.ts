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
export async function copyToClipboard(text: string): Promise<CopyResult> {
	if (!text) {
		return { success: true };
	}

	try {
		// Try the modern Clipboard API first
		// This requires a secure context (HTTPS or localhost) and user interaction in some browsers
		if (navigator.clipboard && window.isSecureContext) {
			await navigator.clipboard.writeText(text);
			return { success: true };
		}

		throw new Error('Clipboard API unavailable');
	} catch (err) {
		// Fallback to document.execCommand('copy')
		// This is deprecated but still necessary for broader support and non-secure contexts
		try {
			const textArea = document.createElement('textarea');
			textArea.value = text;

			// Ensure the element is not visible but part of the DOM (strict-CSP safe)
			// Requires the `.gr-offscreen-input` utility class from Greater CSS.
			textArea.className = 'gr-offscreen-input';
			textArea.setAttribute('readonly', '');
			textArea.setAttribute('aria-hidden', 'true');
			textArea.tabIndex = -1;

			document.body.appendChild(textArea);
			textArea.focus();
			textArea.select();

			const success = document.execCommand('copy');
			document.body.removeChild(textArea);

			if (success) {
				return { success: true };
			} else {
				return { success: false, error: 'execCommand failed' };
			}
		} catch {
			return {
				success: false,
				error: err instanceof Error ? err.message : 'Failed to copy to clipboard',
			};
		}
	}
}

/**
 * Copies the text content of a specific DOM element.
 *
 * @param element - The HTMLElement to copy text from
 * @returns Promise resolving to the result of the operation
 */
export async function copyElementText(element: HTMLElement): Promise<CopyResult> {
	if (!element) {
		return { success: false, error: 'Element not found' };
	}
	return copyToClipboard(element.innerText || element.textContent || '');
}

/**
 * Copies code from a code block, stripping any extra HTML artifacts if necessary.
 * Ideal for <pre><code>...</code></pre> structures.
 *
 * @param codeElement - The HTMLElement containing the code
 * @returns Promise resolving to the result of the operation
 */
export async function copyCodeBlock(codeElement: HTMLElement): Promise<CopyResult> {
	if (!codeElement) {
		return { success: false, error: 'Code element not found' };
	}

	// Clone the element to manipulate it without affecting the display
	const clone = codeElement.cloneNode(true) as HTMLElement;

	// Remove any line numbers or other non-code elements if they exist
	// (This is a common pattern in code block renderers)
	const lineNumbers = clone.querySelectorAll('.line-number, .linenumber');
	lineNumbers.forEach((el) => el.remove());

	const text = clone.innerText || clone.textContent || '';

	// Trim leading/trailing whitespace often found in code blocks
	return copyToClipboard(text.trim());
}
