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
 * Normalize key names for consistency
 */
const KEY_ALIASES: Record<string, string> = {
	cmd: 'meta',
	command: 'meta',
	ctrl: 'control',
	opt: 'alt',
	option: 'alt',
	esc: 'escape',
	del: 'delete',
	ins: 'insert',
	pageup: 'pageup',
	pagedown: 'pagedown',
	space: ' ',
	plus: '+',
	minus: '-',
};

/**
 * Parse a shortcut string into components
 */
function parseShortcut(keys: string): {
	modifiers: Set<string>;
	key: string;
} {
	const parts = keys
		.toLowerCase()
		.split('+')
		.map((p) => p.trim());
	const modifiers = new Set<string>();
	let key = '';

	for (const part of parts) {
		const normalized = KEY_ALIASES[part] || part;

		if (['meta', 'control', 'alt', 'shift'].includes(normalized)) {
			modifiers.add(normalized);
		} else {
			key = normalized;
		}
	}

	return { modifiers, key };
}

/**
 * Check if an element is an input element
 */
function isInputElement(element: EventTarget | null): boolean {
	if (!element || !(element instanceof HTMLElement)) return false;

	const tagName = element.tagName.toLowerCase();
	return (
		tagName === 'input' ||
		tagName === 'textarea' ||
		tagName === 'select' ||
		element.contentEditable === 'true'
	);
}

/**
 * Keyboard shortcut manager
 */
export class KeyboardShortcutManager {
	private shortcuts: Map<string, Shortcut> = new Map();
	private listener: ((event: KeyboardEvent) => void) | null = null;
	private target: HTMLElement | Window;
	private debug: boolean;

	constructor(options: ShortcutManagerOptions = {}) {
		this.target = options.target || window;
		this.debug = options.debug || false;
	}

	/**
	 * Register a keyboard shortcut
	 */
	register(id: string, shortcut: Shortcut): void {
		this.shortcuts.set(id, shortcut);

		if (this.debug) {
			console.warn(`Registered shortcut: ${id} (${shortcut.keys})`);
		}

		// Attach listener if this is the first shortcut
		if (this.shortcuts.size === 1) {
			this.attachListener();
		}
	}

	/**
	 * Unregister a keyboard shortcut
	 */
	unregister(id: string): void {
		this.shortcuts.delete(id);

		if (this.debug) {
			console.warn(`Unregistered shortcut: ${id}`);
		}

		// Remove listener if no shortcuts remain
		if (this.shortcuts.size === 0) {
			this.detachListener();
		}
	}

	/**
	 * Enable or disable a shortcut
	 */
	setEnabled(id: string, enabled: boolean): void {
		const shortcut = this.shortcuts.get(id);
		if (shortcut) {
			shortcut.enabled = enabled;
		}
	}

	/**
	 * Get all registered shortcuts
	 */
	getShortcuts(): Map<string, Shortcut> {
		return new Map(this.shortcuts);
	}

	/**
	 * Clear all shortcuts
	 */
	clear(): void {
		this.shortcuts.clear();
		this.detachListener();
	}

	/**
	 * Destroy the manager and clean up
	 */
	destroy(): void {
		this.clear();
	}

	private attachListener(): void {
		if (this.listener) return;

		this.listener = (event: KeyboardEvent) => {
			this.handleKeyDown(event);
		};

		this.target.addEventListener('keydown', this.listener as EventListener);
	}

	private detachListener(): void {
		if (!this.listener) return;

		this.target.removeEventListener('keydown', this.listener as EventListener);
		this.listener = null;
	}

	private handleKeyDown(event: KeyboardEvent): void {
		const isInput = isInputElement(event.target);

		for (const [id, shortcut] of this.shortcuts) {
			if (shortcut.enabled === false) continue;
			if (isInput && !shortcut.allowInInput) continue;

			if (this.matchesShortcut(event, shortcut)) {
				if (this.debug) {
					console.warn(`Triggered shortcut: ${id}`);
				}

				if (shortcut.preventDefault !== false) {
					event.preventDefault();
				}

				if (shortcut.stopPropagation) {
					event.stopPropagation();
				}

				shortcut.callback(event);
				break; // Only trigger one shortcut per event
			}
		}
	}

	private matchesShortcut(event: KeyboardEvent, shortcut: Shortcut): boolean {
		const { modifiers, key } = parseShortcut(shortcut.keys);

		// Check key
		const eventKey = event.key.toLowerCase();
		if (eventKey !== key && KEY_ALIASES[eventKey] !== key) {
			return false;
		}

		// Check modifiers
		const hasCtrl = event.ctrlKey || event.metaKey;
		const hasMeta = event.metaKey;
		const hasAlt = event.altKey;
		const hasShift = event.shiftKey;

		if (modifiers.has('control') !== hasCtrl && !modifiers.has('meta')) return false;
		if (modifiers.has('meta') !== hasMeta && !modifiers.has('control')) return false;
		if (modifiers.has('alt') !== hasAlt) return false;
		if (modifiers.has('shift') !== hasShift) return false;

		return true;
	}
}

/**
 * Create a keyboard shortcut manager with common defaults
 */
export function createShortcutManager(
	shortcuts?: Record<string, Shortcut>,
	options?: ShortcutManagerOptions
): KeyboardShortcutManager {
	const manager = new KeyboardShortcutManager(options);

	if (shortcuts) {
		for (const [id, shortcut] of Object.entries(shortcuts)) {
			manager.register(id, shortcut);
		}
	}

	return manager;
}

/**
 * Format a shortcut for display (e.g., "Ctrl+S" or "⌘K")
 */
export function formatShortcut(keys: string, style: 'long' | 'short' = 'long'): string {
	const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
	const { modifiers, key } = parseShortcut(keys);

	const parts: string[] = [];

	if (style === 'short' && isMac) {
		if (modifiers.has('control')) parts.push('⌃');
		if (modifiers.has('meta')) parts.push('⌘');
		if (modifiers.has('alt')) parts.push('⌥');
		if (modifiers.has('shift')) parts.push('⇧');
		parts.push(key.toUpperCase());
		return parts.join('');
	}

	if (modifiers.has('control')) parts.push(isMac ? 'Ctrl' : 'Ctrl');
	if (modifiers.has('meta')) parts.push(isMac ? 'Cmd' : 'Win');
	if (modifiers.has('alt')) parts.push(isMac ? 'Option' : 'Alt');
	if (modifiers.has('shift')) parts.push('Shift');
	parts.push(key.toUpperCase());

	return parts.join('+');
}

/**
 * Get platform-appropriate shortcut keys
 */
export function getPlatformShortcut(winKeys: string, macKeys?: string): string {
	const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
	return isMac && macKeys ? macKeys : winKeys;
}
