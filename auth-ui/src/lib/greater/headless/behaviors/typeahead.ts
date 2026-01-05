/**
 * Typeahead Behavior
 *
 * Framework-agnostic utility for type-to-search functionality in menus and lists.
 * Allows users to quickly navigate to items by typing.
 *
 * @module @equaltoai/greater-components-headless/behaviors/typeahead
 */

/**
 * Match mode for typeahead
 */
export type TypeaheadMatchMode = 'prefix' | 'substring';

/**
 * Typeahead configuration
 */
export interface TypeaheadConfig {
	/**
	 * Timeout in ms before search buffer clears
	 * @default 500
	 */
	timeout?: number;

	/**
	 * Match mode
	 * - 'prefix': Match items starting with search string
	 * - 'substring': Match items containing search string
	 * @default 'prefix'
	 */
	matchMode?: TypeaheadMatchMode;

	/**
	 * Whether matching is case-sensitive
	 * @default false
	 */
	caseSensitive?: boolean;

	/**
	 * Function to get searchable text from an item
	 * @default element.textContent
	 */
	getItemText?: (element: HTMLElement, index: number) => string;

	/**
	 * Function to determine if an item is disabled
	 */
	isDisabled?: (element: HTMLElement, index: number) => boolean;

	/**
	 * Callback when a match is found
	 */
	onMatch?: (index: number, element: HTMLElement) => void;

	/**
	 * Callback when no match is found
	 */
	onNoMatch?: (searchString: string) => void;

	/**
	 * Callback when search buffer changes
	 */
	onBufferChange?: (buffer: string) => void;
}

/**
 * Typeahead state
 */
export interface TypeaheadState {
	/**
	 * Current search buffer
	 */
	buffer: string;

	/**
	 * Whether typeahead is active
	 */
	active: boolean;

	/**
	 * Last matched index
	 */
	lastMatchIndex: number;
}

/**
 * Typeahead return type
 */
export interface Typeahead {
	/**
	 * Current state
	 */
	state: TypeaheadState;

	/**
	 * Handle a character input
	 */
	handleChar: (char: string, items: HTMLElement[]) => number;

	/**
	 * Handle keyboard event
	 */
	handleKeyDown: (event: KeyboardEvent, items: HTMLElement[]) => boolean;

	/**
	 * Clear the search buffer
	 */
	clear: () => void;

	/**
	 * Search for a string
	 */
	search: (searchString: string, items: HTMLElement[]) => number;

	/**
	 * Update configuration
	 */
	updateConfig: (config: Partial<TypeaheadConfig>) => void;

	/**
	 * Destroy and clean up
	 */
	destroy: () => void;
}

/**
 * Default text getter
 */
function defaultGetItemText(element: HTMLElement): string {
	return element.textContent?.trim() || '';
}

/**
 * Default disabled check
 */
function defaultIsDisabled(element: HTMLElement): boolean {
	return (
		element.hasAttribute('disabled') ||
		element.getAttribute('aria-disabled') === 'true' ||
		element.hasAttribute('data-disabled')
	);
}

/**
 * Create a typeahead manager
 */
export function createTypeahead(config: TypeaheadConfig = {}): Typeahead {
	let currentConfig: Required<TypeaheadConfig> = {
		timeout: config.timeout ?? 500,
		matchMode: config.matchMode ?? 'prefix',
		caseSensitive: config.caseSensitive ?? false,
		getItemText: config.getItemText ?? defaultGetItemText,
		isDisabled: config.isDisabled ?? defaultIsDisabled,
		onMatch: config.onMatch ?? (() => {}),
		onNoMatch: config.onNoMatch ?? (() => {}),
		onBufferChange: config.onBufferChange ?? (() => {}),
	};

	const state: TypeaheadState = {
		buffer: '',
		active: true,
		lastMatchIndex: -1,
	};

	let timeoutId: ReturnType<typeof setTimeout> | undefined;

	/**
	 * Clear the search buffer
	 */
	function clear(): void {
		if (timeoutId !== undefined) {
			clearTimeout(timeoutId);
			timeoutId = undefined;
		}
		state.buffer = '';
		currentConfig.onBufferChange('');
	}

	/**
	 * Reset timeout
	 */
	function resetTimeout(): void {
		if (timeoutId !== undefined) {
			clearTimeout(timeoutId);
		}
		timeoutId = setTimeout(() => {
			clear();
		}, currentConfig.timeout);
	}

	/**
	 * Normalize string for comparison
	 */
	function normalize(str: string): string {
		return currentConfig.caseSensitive ? str : str.toLowerCase();
	}

	/**
	 * Check if item text matches search string
	 */
	function matches(itemText: string, searchString: string): boolean {
		const normalizedItem = normalize(itemText);
		const normalizedSearch = normalize(searchString);

		if (currentConfig.matchMode === 'prefix') {
			return normalizedItem.startsWith(normalizedSearch);
		}
		return normalizedItem.includes(normalizedSearch);
	}

	/**
	 * Search for a string in items
	 */
	function search(searchString: string, items: HTMLElement[]): number {
		if (!searchString || items.length === 0) return -1;

		// Start searching from the item after the last match
		const startIndex = state.lastMatchIndex >= 0 ? state.lastMatchIndex + 1 : 0;

		// First pass: search from startIndex to end
		for (let i = startIndex; i < items.length; i++) {
			const item = items[i];
			if (!item) continue;
			if (currentConfig.isDisabled(item, i)) continue;

			const text = currentConfig.getItemText(item, i);
			if (matches(text, searchString)) {
				state.lastMatchIndex = i;
				currentConfig.onMatch(i, item);
				return i;
			}
		}

		// Second pass: wrap around from beginning to startIndex
		for (let i = 0; i < startIndex; i++) {
			const item = items[i];
			if (!item) continue;
			if (currentConfig.isDisabled(item, i)) continue;

			const text = currentConfig.getItemText(item, i);
			if (matches(text, searchString)) {
				state.lastMatchIndex = i;
				currentConfig.onMatch(i, item);
				return i;
			}
		}

		currentConfig.onNoMatch(searchString);
		return -1;
	}

	/**
	 * Handle a character input
	 */
	function handleChar(char: string, items: HTMLElement[]): number {
		if (!state.active) return -1;

		// Only handle single printable characters
		if (char.length !== 1) return -1;

		// Add to buffer
		state.buffer += char;
		currentConfig.onBufferChange(state.buffer);

		// Reset timeout
		resetTimeout();

		// Search for match
		return search(state.buffer, items);
	}

	/**
	 * Handle keyboard event
	 * Returns true if the event was handled
	 */
	function handleKeyDown(event: KeyboardEvent, items: HTMLElement[]): boolean {
		if (!state.active) return false;

		// Ignore if modifier keys are pressed (except Shift for uppercase)
		if (event.ctrlKey || event.metaKey || event.altKey) {
			return false;
		}

		// Handle printable characters
		if (event.key.length === 1) {
			const result = handleChar(event.key, items);
			return result !== -1;
		}

		// Handle Escape to clear buffer
		if (event.key === 'Escape' && state.buffer.length > 0) {
			clear();
			return true;
		}

		// Handle Backspace to remove last character
		if (event.key === 'Backspace' && state.buffer.length > 0) {
			state.buffer = state.buffer.slice(0, -1);
			currentConfig.onBufferChange(state.buffer);
			resetTimeout();

			if (state.buffer.length > 0) {
				// Re-search with shorter buffer
				state.lastMatchIndex = -1; // Reset to search from beginning
				search(state.buffer, items);
			}
			return true;
		}

		return false;
	}

	/**
	 * Update configuration
	 */
	function updateConfig(newConfig: Partial<TypeaheadConfig>): void {
		currentConfig = {
			...currentConfig,
			...newConfig,
			getItemText: newConfig.getItemText ?? currentConfig.getItemText,
			isDisabled: newConfig.isDisabled ?? currentConfig.isDisabled,
			onMatch: newConfig.onMatch ?? currentConfig.onMatch,
			onNoMatch: newConfig.onNoMatch ?? currentConfig.onNoMatch,
			onBufferChange: newConfig.onBufferChange ?? currentConfig.onBufferChange,
		};
	}

	/**
	 * Destroy and clean up
	 */
	function destroy(): void {
		clear();
		state.active = false;
	}

	return {
		state,
		handleChar,
		handleKeyDown,
		clear,
		search,
		updateConfig,
		destroy,
	};
}
