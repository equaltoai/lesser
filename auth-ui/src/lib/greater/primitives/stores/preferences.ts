// Type definitions for preferences
export type ColorScheme = 'light' | 'dark' | 'high-contrast' | 'auto';
export type Density = 'compact' | 'comfortable' | 'spacious';
export type FontSize = 'small' | 'medium' | 'large';
export type MotionPreference = 'normal' | 'reduced';

export interface ThemeColors {
	primary: string;
	secondary: string;
	accent?: string;
}

export interface UserPreferences {
	colorScheme: ColorScheme;
	density: Density;
	fontSize: FontSize;
	motion: MotionPreference;
	customColors: ThemeColors;
	highContrastMode: boolean;
	fontScale: number; // 0.85 to 1.5 multiplier
}

export interface PreferencesState extends UserPreferences {
	systemColorScheme: 'light' | 'dark';
	systemMotion: MotionPreference;
	systemHighContrast: boolean;
	resolvedColorScheme: 'light' | 'dark' | 'high-contrast';
}

// Local storage key
const PREFERENCES_KEY = 'gr-preferences-v1';

// Default preferences
const DEFAULT_PREFERENCES: UserPreferences = {
	colorScheme: 'auto',
	density: 'comfortable',
	fontSize: 'medium',
	motion: 'normal',
	customColors: {
		primary: '#3b82f6',
		secondary: '#8b5cf6',
		accent: '#ec4899',
	},
	highContrastMode: false,
	fontScale: 1,
};

function clonePreferences(preferences: UserPreferences): UserPreferences {
	return {
		...preferences,
		customColors: {
			...preferences.customColors,
		},
	};
}

function mergePreferences(
	base: UserPreferences,
	updates: Partial<UserPreferences>
): UserPreferences {
	const next = {
		...base,
		...updates,
	};

	next.customColors = {
		...base.customColors,
		...(updates.customColors ?? {}),
	};

	return clonePreferences(next);
}

// Create the preferences store class
class PreferencesStore {
	// Internal state
	private _preferences: UserPreferences = clonePreferences(DEFAULT_PREFERENCES);
	private _systemColorScheme: 'light' | 'dark' = 'light';
	private _systemMotion: MotionPreference = 'normal';
	private _systemHighContrast = false;

	// Media query matchers
	private darkModeQuery?: MediaQueryList;
	private reducedMotionQuery?: MediaQueryList;
	private highContrastQuery?: MediaQueryList;

	constructor() {
		// Initialize from localStorage
		this.loadPreferences();

		// Set up system preference detection
		this.setupSystemPreferenceDetection();

		// Apply initial theme
		this.applyTheme();
	}

	// Getters using $derived for computed values
	get preferences(): UserPreferences {
		return clonePreferences(this._preferences);
	}

	get state(): PreferencesState {
		const { customColors, ...rest } = this._preferences;
		return {
			...rest,
			customColors: {
				...customColors,
			},
			systemColorScheme: this._systemColorScheme,
			systemMotion: this._systemMotion,
			systemHighContrast: this._systemHighContrast,
			resolvedColorScheme: this.resolvedColorScheme,
		};
	}

	get resolvedColorScheme(): 'light' | 'dark' | 'high-contrast' {
		// High contrast overrides everything
		if (this._preferences.highContrastMode || this._systemHighContrast) {
			return 'high-contrast';
		}

		// Handle auto color scheme
		if (this._preferences.colorScheme === 'auto') {
			return this._systemColorScheme;
		}

		// Use explicit user preference
		if (this._preferences.colorScheme === 'high-contrast') {
			return 'high-contrast';
		}

		return this._preferences.colorScheme as 'light' | 'dark';
	}

	get resolvedMotion(): MotionPreference {
		// System preference for reduced motion always wins
		if (this._systemMotion === 'reduced') {
			return 'reduced';
		}
		return this._preferences.motion;
	}

	// Methods to update preferences
	setColorScheme(scheme: ColorScheme) {
		this._preferences.colorScheme = scheme;
		this.saveAndApply();
	}

	setDensity(density: Density) {
		this._preferences.density = density;
		this.saveAndApply();
	}

	setFontSize(size: FontSize) {
		this._preferences.fontSize = size;
		this.saveAndApply();
	}

	setFontScale(scale: number) {
		// Clamp between 0.85 and 1.5
		this._preferences.fontScale = Math.max(0.85, Math.min(1.5, scale));
		this.saveAndApply();
	}

	setMotion(motion: MotionPreference) {
		this._preferences.motion = motion;
		this.saveAndApply();
	}

	setCustomColors(colors: Partial<ThemeColors>) {
		this._preferences.customColors = {
			...this._preferences.customColors,
			...colors,
		};
		this.saveAndApply();
	}

	setHighContrastMode(enabled: boolean) {
		this._preferences.highContrastMode = enabled;
		this.saveAndApply();
	}

	updatePreferences(updates: Partial<UserPreferences>) {
		this._preferences = mergePreferences(this._preferences, updates);
		this.saveAndApply();
	}

	// Reset to defaults
	reset() {
		this._preferences = clonePreferences(DEFAULT_PREFERENCES);
		this.saveAndApply();
	}

	// Export current preferences as JSON
	export(): string {
		return JSON.stringify(this._preferences, null, 2);
	}

	// Import preferences from JSON
	import(json: string): boolean {
		try {
			const imported = JSON.parse(json) as Partial<UserPreferences>;

			// Validate imported data
			if (this.validatePreferences(imported)) {
				this._preferences = mergePreferences(DEFAULT_PREFERENCES, imported);
				this.saveAndApply();
				return true;
			}
			return false;
		} catch {
			return false;
		}
	}

	// Private methods
	private loadPreferences() {
		if (typeof window === 'undefined') return;

		try {
			const stored = localStorage.getItem(PREFERENCES_KEY);
			if (stored) {
				const parsed = JSON.parse(stored) as Partial<UserPreferences>;
				if (this.validatePreferences(parsed)) {
					this._preferences = mergePreferences(DEFAULT_PREFERENCES, parsed);
				}
			}
		} catch (error) {
			console.warn('Failed to load preferences from localStorage:', error);
		}
	}

	private savePreferences() {
		if (typeof window === 'undefined') return;

		try {
			localStorage.setItem(PREFERENCES_KEY, JSON.stringify(this._preferences));
		} catch (error) {
			console.warn('Failed to save preferences to localStorage:', error);
		}
	}

	private saveAndApply() {
		this.savePreferences();
		this.applyTheme();
	}

	private validatePreferences(prefs: Partial<UserPreferences>): boolean {
		// Basic validation of preference values
		const validColorSchemes = ['light', 'dark', 'high-contrast', 'auto'];
		const validDensities = ['compact', 'comfortable', 'spacious'];
		const validFontSizes = ['small', 'medium', 'large'];
		const validMotion = ['normal', 'reduced'];

		if (prefs.colorScheme && !validColorSchemes.includes(prefs.colorScheme)) {
			return false;
		}
		if (prefs.density && !validDensities.includes(prefs.density)) {
			return false;
		}
		if (prefs.fontSize && !validFontSizes.includes(prefs.fontSize)) {
			return false;
		}
		if (prefs.motion && !validMotion.includes(prefs.motion)) {
			return false;
		}

		return true;
	}

	private setupSystemPreferenceDetection() {
		if (typeof window === 'undefined') return;

		// Dark mode detection
		this.darkModeQuery = window.matchMedia('(prefers-color-scheme: dark)');
		this._systemColorScheme = this.darkModeQuery.matches ? 'dark' : 'light';

		this.darkModeQuery.addEventListener('change', (e) => {
			this._systemColorScheme = e.matches ? 'dark' : 'light';
			if (this._preferences.colorScheme === 'auto') {
				this.applyTheme();
			}
		});

		// Reduced motion detection
		this.reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		this._systemMotion = this.reducedMotionQuery.matches ? 'reduced' : 'normal';

		this.reducedMotionQuery.addEventListener('change', (e) => {
			this._systemMotion = e.matches ? 'reduced' : 'normal';
			this.applyTheme();
		});

		// High contrast detection
		this.highContrastQuery = window.matchMedia('(prefers-contrast: high)');
		this._systemHighContrast = this.highContrastQuery.matches;

		this.highContrastQuery.addEventListener('change', (e) => {
			this._systemHighContrast = e.matches;
			this.applyTheme();
		});
	}

	private applyTheme() {
		if (typeof window === 'undefined') return;

		const root = document.documentElement;

		// Apply color scheme
		root.setAttribute('data-theme', this.resolvedColorScheme);

		// Apply density
		root.setAttribute('data-density', this._preferences.density);

		// Apply font size
		root.setAttribute('data-font-size', this._preferences.fontSize);

		// Apply motion preference
		root.setAttribute('data-motion', this.resolvedMotion);

		// Apply custom CSS variables
		this.applyCustomProperties();
	}

	private applyCustomProperties() {
		if (typeof window === 'undefined') return;

		const root = document.documentElement;

		// Apply custom colors
		if (this._preferences.customColors.primary) {
			root.style.setProperty('--gr-custom-primary', this._preferences.customColors.primary);
		}
		if (this._preferences.customColors.secondary) {
			root.style.setProperty('--gr-custom-secondary', this._preferences.customColors.secondary);
		}
		if (this._preferences.customColors.accent) {
			root.style.setProperty('--gr-custom-accent', this._preferences.customColors.accent);
		}

		// Apply font scale
		root.style.setProperty('--gr-font-scale', String(this._preferences.fontScale));

		// Apply density-based spacing multiplier
		const densityScale = {
			compact: 0.85,
			comfortable: 1,
			spacious: 1.2,
		};
		root.style.setProperty('--gr-density-scale', String(densityScale[this._preferences.density]));
	}

	// Cleanup method
	destroy() {
		// Remove event listeners if they exist
		if (this.darkModeQuery) {
			// Note: We stored the handlers inline, so we can't remove them
			// In a production app, we'd store references to the handlers
		}
	}
}

// Export singleton instance
export const preferencesStore = new PreferencesStore();

// Export convenience functions for use in components
export function getPreferences() {
	return preferencesStore.preferences;
}

export function getPreferenceState() {
	return preferencesStore.state;
}
