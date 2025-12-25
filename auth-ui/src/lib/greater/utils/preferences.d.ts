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
export declare function createPreferenceStore<T extends Record<string, unknown>>(
	key: string,
	defaults: T
): PreferenceStore<T>;
//# sourceMappingURL=preferences.d.ts.map
