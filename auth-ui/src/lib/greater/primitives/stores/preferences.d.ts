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
	fontScale: number;
}
export interface PreferencesState extends UserPreferences {
	systemColorScheme: 'light' | 'dark';
	systemMotion: MotionPreference;
	systemHighContrast: boolean;
	resolvedColorScheme: 'light' | 'dark' | 'high-contrast';
}
export interface ThemeBootstrapSnapshot {
	preferences: UserPreferences;
	systemColorScheme: 'light' | 'dark';
	systemMotion: MotionPreference;
	systemHighContrast: boolean;
}
export interface ThemeBootstrapOptions {
	defaults?: Partial<UserPreferences>;
	stored?: Partial<UserPreferences>;
	cookie?: string | Record<string, string | undefined>;
	cookieName?: string;
	system?: Partial<{
		colorScheme: 'light' | 'dark';
		motion: MotionPreference;
		highContrast: boolean;
	}>;
}
export interface ThemeDocumentAttributes {
	'data-theme': string;
	'data-density': Density;
	'data-font-size': FontSize;
	'data-motion': MotionPreference;
	'data-gr-font-scale': string;
	'data-gr-custom-primary'?: string;
	'data-gr-custom-secondary'?: string;
	'data-gr-custom-accent'?: string;
}
export declare const PREFERENCES_KEY = 'gr-preferences-v1';
export declare const PREFERENCES_COOKIE = 'gr-preferences';
export declare function parsePreferencesCookie(
	input: string | Record<string, string | undefined> | undefined,
	cookieName?: string
): Partial<UserPreferences> | null;
export declare function createThemeBootstrapSnapshot(
	options?: ThemeBootstrapOptions
): ThemeBootstrapSnapshot;
export declare function getThemeBootstrapState(snapshot: ThemeBootstrapSnapshot): PreferencesState;
export declare function getThemeDocumentAttributes(
	snapshot: ThemeBootstrapSnapshot
): ThemeDocumentAttributes;
export declare function applyThemeDocumentAttributes(
	snapshot: ThemeBootstrapSnapshot,
	root?: HTMLElement | null
): void;
export declare function readThemeBootstrapSnapshotFromDocument(
	root?: HTMLElement | null
): ThemeBootstrapSnapshot | null;
declare class PreferencesStore {
	private _preferences;
	private _systemColorScheme;
	private _systemMotion;
	private _systemHighContrast;
	private _initialized;
	private _systemPreferenceDetectionReady;
	private darkModeQuery?;
	private reducedMotionQuery?;
	private highContrastQuery?;
	private ensureInitialized;
	private applySnapshot;
	get preferences(): UserPreferences;
	get state(): PreferencesState;
	get resolvedColorScheme(): 'light' | 'dark' | 'high-contrast';
	get resolvedMotion(): MotionPreference;
	hydrate(
		snapshot: ThemeBootstrapSnapshot,
		options?: {
			persist?: boolean;
		}
	): void;
	setColorScheme(scheme: ColorScheme): void;
	setDensity(density: Density): void;
	setFontSize(size: FontSize): void;
	setFontScale(scale: number): void;
	setMotion(motion: MotionPreference): void;
	setCustomColors(colors: Partial<ThemeColors>): void;
	setHighContrastMode(enabled: boolean): void;
	updatePreferences(updates: Partial<UserPreferences>): void;
	reset(): void;
	export(): string;
	import(json: string): boolean;
	private loadPreferences;
	private savePreferences;
	private saveAndApply;
	private setupSystemPreferenceDetection;
	private applyTheme;
	destroy(): void;
}
export declare const preferencesStore: PreferencesStore;
export declare function getPreferences(): UserPreferences;
export declare function getPreferenceState(): PreferencesState;
export {};
//# sourceMappingURL=preferences.d.ts.map
