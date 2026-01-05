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
declare class PreferencesStore {
	private _preferences;
	private _systemColorScheme;
	private _systemMotion;
	private _systemHighContrast;
	private darkModeQuery?;
	private reducedMotionQuery?;
	private highContrastQuery?;
	constructor();
	get preferences(): UserPreferences;
	get state(): PreferencesState;
	get resolvedColorScheme(): 'light' | 'dark' | 'high-contrast';
	get resolvedMotion(): MotionPreference;
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
	private validatePreferences;
	private setupSystemPreferenceDetection;
	private applyTheme;
	private applyCustomProperties;
	destroy(): void;
}
export declare const preferencesStore: PreferencesStore;
export declare function getPreferences(): UserPreferences;
export declare function getPreferenceState(): PreferencesState;
export {};
//# sourceMappingURL=preferences.d.ts.map
