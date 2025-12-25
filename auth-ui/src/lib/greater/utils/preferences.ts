/**
 * Preference Store Utility
 *
 * Provides a standardized way to manage user preferences with persistence.
 */

export interface PreferenceStore<T extends Record<string, unknown>> {
	/**
	 * Get current preferences
	 */
	get(): T;

	/**
	 * Update single preference
	 */
	set<K extends keyof T>(key: K, value: T[K]): void;

	/**
	 * Update multiple preferences
	 */
	update(partial: Partial<T>): void;

	/**
	 * Reset to defaults
	 */
	reset(): void;

	/**
	 * Subscribe to changes
	 */
	subscribe(callback: (prefs: T) => void): () => void;

	/**
	 * Export preferences as JSON
	 */
	export(): string;

	/**
	 * Import preferences from JSON
	 */
	import(json: string): boolean;
}

/**
 * Create a preference store with localStorage persistence
 */
export function createPreferenceStore<T extends Record<string, unknown>>(
	key: string,
	defaults: T
): PreferenceStore<T> {
	let state = { ...defaults };
	const subscribers = new Set<(prefs: T) => void>();

	// Load from storage
	if (typeof localStorage !== 'undefined') {
		try {
			const stored = localStorage.getItem(key);
			if (stored) {
				const parsed = JSON.parse(stored);
				state = { ...state, ...parsed };
			}
		} catch (e) {
			console.error('Failed to load preferences', e);
		}
	}

	const notify = () => {
		for (const sub of subscribers) {
			sub(state);
		}
		save();
	};

	const save = () => {
		if (typeof localStorage !== 'undefined') {
			try {
				localStorage.setItem(key, JSON.stringify(state));
			} catch (e) {
				console.error('Failed to save preferences', e);
			}
		}
	};

	return {
		get: () => state,
		set: (k, v) => {
			state = { ...state, [k]: v };
			notify();
		},
		update: (partial) => {
			state = { ...state, ...partial };
			notify();
		},
		reset: () => {
			state = { ...defaults };
			notify();
		},
		subscribe: (cb) => {
			subscribers.add(cb);
			cb(state);
			return () => {
				subscribers.delete(cb);
			};
		},
		export: () => JSON.stringify(state),
		import: (json) => {
			try {
				const parsed = JSON.parse(json);
				state = { ...state, ...parsed };
				notify();
				return true;
			} catch {
				return false;
			}
		},
	};
}
