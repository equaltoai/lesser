export interface ShortcutOptions {
	/**
	 * Whether to prevent default browser behavior
	 */
	preventDefault?: boolean;
	/**
	 * Whether to stop event propagation
	 */
	stopPropagation?: boolean;
	/**
	 * Whether to only trigger in specific input contexts
	 */
	allowInInput?: boolean;
	/**
	 * Whether the shortcut is enabled
	 */
	enabled?: boolean;
	/**
	 * Description of the shortcut (for help text)
	 */
	description?: string;
}
export interface Shortcut extends ShortcutOptions {
	/**
	 * The key combination (e.g., "ctrl+s", "cmd+k", "shift+?")
	 */
	keys: string;
	/**
	 * The callback to execute
	 */
	callback: (event: KeyboardEvent) => void;
}
export interface ShortcutManagerOptions {
	/**
	 * Enable debug logging
	 */
	debug?: boolean;
	/**
	 * Target element to attach listeners to
	 */
	target?: HTMLElement | Window;
}
/**
 * Keyboard shortcut manager
 */
export declare class KeyboardShortcutManager {
	private shortcuts;
	private listener;
	private target;
	private debug;
	constructor(options?: ShortcutManagerOptions);
	/**
	 * Register a keyboard shortcut
	 */
	register(id: string, shortcut: Shortcut): void;
	/**
	 * Unregister a keyboard shortcut
	 */
	unregister(id: string): void;
	/**
	 * Enable or disable a shortcut
	 */
	setEnabled(id: string, enabled: boolean): void;
	/**
	 * Get all registered shortcuts
	 */
	getShortcuts(): Map<string, Shortcut>;
	/**
	 * Clear all shortcuts
	 */
	clear(): void;
	/**
	 * Destroy the manager and clean up
	 */
	destroy(): void;
	private attachListener;
	private detachListener;
	private handleKeyDown;
	private matchesShortcut;
}
/**
 * Create a keyboard shortcut manager with common defaults
 */
export declare function createShortcutManager(
	shortcuts?: Record<string, Shortcut>,
	options?: ShortcutManagerOptions
): KeyboardShortcutManager;
/**
 * Format a shortcut for display (e.g., "Ctrl+S" or "⌘K")
 */
export declare function formatShortcut(keys: string, style?: 'long' | 'short'): string;
/**
 * Get platform-appropriate shortcut keys
 */
export declare function getPlatformShortcut(winKeys: string, macKeys?: string): string;
//# sourceMappingURL=keyboardShortcuts.d.ts.map
