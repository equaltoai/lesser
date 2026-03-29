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

export const PREFERENCES_KEY = 'gr-preferences-v1';
export const PREFERENCES_COOKIE = 'gr-preferences';

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

const VALID_COLOR_SCHEMES: ColorScheme[] = ['light', 'dark', 'high-contrast', 'auto'];
const VALID_DENSITIES: Density[] = ['compact', 'comfortable', 'spacious'];
const VALID_FONT_SIZES: FontSize[] = ['small', 'medium', 'large'];
const VALID_MOTION_PREFERENCES: MotionPreference[] = ['normal', 'reduced'];

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

function isColorScheme(value: unknown): value is ColorScheme {
	return typeof value === 'string' && VALID_COLOR_SCHEMES.includes(value as ColorScheme);
}

function isDensity(value: unknown): value is Density {
	return typeof value === 'string' && VALID_DENSITIES.includes(value as Density);
}

function isFontSize(value: unknown): value is FontSize {
	return typeof value === 'string' && VALID_FONT_SIZES.includes(value as FontSize);
}

function isMotionPreference(value: unknown): value is MotionPreference {
	return typeof value === 'string' && VALID_MOTION_PREFERENCES.includes(value as MotionPreference);
}

function validatePreferences(prefs: Partial<UserPreferences>): boolean {
	if (typeof prefs !== 'object' || prefs === null) {
		return false;
	}

	if (typeof prefs.colorScheme !== 'undefined' && !isColorScheme(prefs.colorScheme)) {
		return false;
	}

	if (typeof prefs.density !== 'undefined' && !isDensity(prefs.density)) {
		return false;
	}

	if (typeof prefs.fontSize !== 'undefined' && !isFontSize(prefs.fontSize)) {
		return false;
	}

	if (typeof prefs.motion !== 'undefined' && !isMotionPreference(prefs.motion)) {
		return false;
	}

	if (
		typeof prefs.highContrastMode !== 'undefined' &&
		typeof prefs.highContrastMode !== 'boolean'
	) {
		return false;
	}

	if (typeof prefs.fontScale !== 'undefined' && typeof prefs.fontScale !== 'number') {
		return false;
	}

	return true;
}

function resolveColorScheme(
	preferences: UserPreferences,
	systemColorScheme: 'light' | 'dark',
	systemHighContrast: boolean
): 'light' | 'dark' | 'high-contrast' {
	if (preferences.highContrastMode || systemHighContrast) {
		return 'high-contrast';
	}

	if (preferences.colorScheme === 'auto') {
		return systemColorScheme;
	}

	if (preferences.colorScheme === 'high-contrast') {
		return 'high-contrast';
	}

	return preferences.colorScheme as 'light' | 'dark';
}

function resolveMotion(
	preferences: UserPreferences,
	systemMotion: MotionPreference
): MotionPreference {
	if (systemMotion === 'reduced') {
		return 'reduced';
	}

	return preferences.motion;
}

function parseCookieHeader(header: string): Record<string, string> {
	return header
		.split(';')
		.map((part) => part.trim())
		.filter(Boolean)
		.reduce<Record<string, string>>((cookies, part) => {
			const separatorIndex = part.indexOf('=');
			if (separatorIndex === -1) {
				return cookies;
			}

			const name = part.slice(0, separatorIndex).trim();
			const value = part.slice(separatorIndex + 1).trim();
			if (name) {
				cookies[name] = value;
			}

			return cookies;
		}, {});
}

export function parsePreferencesCookie(
	input: string | Record<string, string | undefined> | undefined,
	cookieName = PREFERENCES_COOKIE
): Partial<UserPreferences> | null {
	if (!input) {
		return null;
	}

	const cookies = typeof input === 'string' ? parseCookieHeader(input) : input;
	const rawValue = cookies[cookieName];
	if (!rawValue) {
		return null;
	}

	try {
		const parsed = JSON.parse(decodeURIComponent(rawValue)) as Partial<UserPreferences>;
		return validatePreferences(parsed) ? parsed : null;
	} catch {
		return null;
	}
}

export function createThemeBootstrapSnapshot(
	options: ThemeBootstrapOptions = {}
): ThemeBootstrapSnapshot {
	const basePreferences = mergePreferences(DEFAULT_PREFERENCES, options.defaults ?? {});
	const cookiePreferences = parsePreferencesCookie(options.cookie, options.cookieName) ?? {};
	const storedPreferences = validatePreferences(options.stored ?? {}) ? (options.stored ?? {}) : {};
	const preferences = mergePreferences(
		mergePreferences(basePreferences, cookiePreferences),
		storedPreferences
	);

	return {
		preferences,
		systemColorScheme: options.system?.colorScheme ?? 'light',
		systemMotion: options.system?.motion ?? 'normal',
		systemHighContrast: options.system?.highContrast ?? false,
	};
}

export function getThemeBootstrapState(snapshot: ThemeBootstrapSnapshot): PreferencesState {
	return {
		...clonePreferences(snapshot.preferences),
		systemColorScheme: snapshot.systemColorScheme,
		systemMotion: snapshot.systemMotion,
		systemHighContrast: snapshot.systemHighContrast,
		resolvedColorScheme: resolveColorScheme(
			snapshot.preferences,
			snapshot.systemColorScheme,
			snapshot.systemHighContrast
		),
	};
}

export function getThemeDocumentAttributes(
	snapshot: ThemeBootstrapSnapshot
): ThemeDocumentAttributes {
	const state = getThemeBootstrapState(snapshot);
	const attributes: ThemeDocumentAttributes = {
		'data-theme': state.resolvedColorScheme,
		'data-density': state.density,
		'data-font-size': state.fontSize,
		'data-motion': resolveMotion(snapshot.preferences, snapshot.systemMotion),
		'data-gr-font-scale': String(snapshot.preferences.fontScale),
	};

	if (snapshot.preferences.customColors.primary) {
		attributes['data-gr-custom-primary'] = snapshot.preferences.customColors.primary;
	}

	if (snapshot.preferences.customColors.secondary) {
		attributes['data-gr-custom-secondary'] = snapshot.preferences.customColors.secondary;
	}

	if (snapshot.preferences.customColors.accent) {
		attributes['data-gr-custom-accent'] = snapshot.preferences.customColors.accent;
	}

	return attributes;
}

export function applyThemeDocumentAttributes(
	snapshot: ThemeBootstrapSnapshot,
	root: HTMLElement | null = typeof document !== 'undefined' ? document.documentElement : null
): void {
	if (!root) {
		return;
	}

	const attributes = getThemeDocumentAttributes(snapshot);
	const attributeNames: Array<keyof ThemeDocumentAttributes> = [
		'data-theme',
		'data-density',
		'data-font-size',
		'data-motion',
		'data-gr-font-scale',
		'data-gr-custom-primary',
		'data-gr-custom-secondary',
		'data-gr-custom-accent',
	];

	attributeNames.forEach((attributeName) => {
		const value = attributes[attributeName];
		if (value) {
			root.setAttribute(attributeName, value);
		} else {
			root.removeAttribute(attributeName);
		}
	});
}

export function readThemeBootstrapSnapshotFromDocument(
	root: HTMLElement | null = typeof document !== 'undefined' ? document.documentElement : null
): ThemeBootstrapSnapshot | null {
	if (!root) {
		return null;
	}

	const theme = root.getAttribute('data-theme');
	const density = root.getAttribute('data-density');
	const fontSize = root.getAttribute('data-font-size');
	const motion = root.getAttribute('data-motion');
	const fontScale = root.getAttribute('data-gr-font-scale');
	const customPrimary = root.getAttribute('data-gr-custom-primary');
	const customSecondary = root.getAttribute('data-gr-custom-secondary');
	const customAccent = root.getAttribute('data-gr-custom-accent');

	if (!theme && !density && !fontSize && !motion && !fontScale) {
		return null;
	}

	const preferences = clonePreferences(DEFAULT_PREFERENCES);

	if (theme === 'high-contrast') {
		preferences.colorScheme = 'high-contrast';
		preferences.highContrastMode = true;
	} else if (theme === 'dark' || theme === 'light') {
		preferences.colorScheme = theme;
	}

	if (density && isDensity(density)) {
		preferences.density = density;
	}

	if (fontSize && isFontSize(fontSize)) {
		preferences.fontSize = fontSize;
	}

	if (motion && isMotionPreference(motion)) {
		preferences.motion = motion;
	}

	if (fontScale) {
		const parsedFontScale = Number(fontScale);
		if (Number.isFinite(parsedFontScale)) {
			preferences.fontScale = parsedFontScale;
		}
	}

	if (customPrimary) {
		preferences.customColors.primary = customPrimary;
	}

	if (customSecondary) {
		preferences.customColors.secondary = customSecondary;
	}

	if (customAccent) {
		preferences.customColors.accent = customAccent;
	}

	return {
		preferences,
		systemColorScheme: theme === 'dark' ? 'dark' : 'light',
		systemMotion: motion === 'reduced' ? 'reduced' : 'normal',
		// `data-theme` captures the resolved theme, not whether the OS is actively forcing high contrast.
		// Treating a resolved high-contrast document as a live system preference locks the user into that
		// mode until a matchMedia change event fires.
		systemHighContrast: false,
	};
}

class PreferencesStore {
	private _preferences: UserPreferences = clonePreferences(DEFAULT_PREFERENCES);
	private _systemColorScheme: 'light' | 'dark' = 'light';
	private _systemMotion: MotionPreference = 'normal';
	private _systemHighContrast = false;
	private _initialized = false;
	private _systemPreferenceDetectionReady = false;
	private darkModeQuery?: MediaQueryList;
	private reducedMotionQuery?: MediaQueryList;
	private highContrastQuery?: MediaQueryList;

	private ensureInitialized() {
		if (this._initialized) {
			return;
		}

		const documentSnapshot = readThemeBootstrapSnapshotFromDocument();
		if (documentSnapshot) {
			this.applySnapshot(documentSnapshot, { apply: false, persist: false });
			this.setupSystemPreferenceDetection({ preserveInitialValues: true });
		} else {
			this.loadPreferences();
			this.setupSystemPreferenceDetection();
		}

		if (typeof window !== 'undefined') {
			this.applyTheme();
		}

		this._initialized = true;
	}

	private applySnapshot(
		snapshot: ThemeBootstrapSnapshot,
		options: { apply: boolean; persist: boolean }
	) {
		this._preferences = clonePreferences(snapshot.preferences);
		this._systemColorScheme = snapshot.systemColorScheme;
		this._systemMotion = snapshot.systemMotion;
		this._systemHighContrast = snapshot.systemHighContrast;

		if (options.persist) {
			this.savePreferences();
		}

		if (options.apply) {
			this.applyTheme();
		}
	}

	get preferences(): UserPreferences {
		this.ensureInitialized();
		return clonePreferences(this._preferences);
	}

	get state(): PreferencesState {
		this.ensureInitialized();
		return getThemeBootstrapState({
			preferences: this._preferences,
			systemColorScheme: this._systemColorScheme,
			systemMotion: this._systemMotion,
			systemHighContrast: this._systemHighContrast,
		});
	}

	get resolvedColorScheme(): 'light' | 'dark' | 'high-contrast' {
		this.ensureInitialized();
		return resolveColorScheme(this._preferences, this._systemColorScheme, this._systemHighContrast);
	}

	get resolvedMotion(): MotionPreference {
		this.ensureInitialized();
		return resolveMotion(this._preferences, this._systemMotion);
	}

	hydrate(snapshot: ThemeBootstrapSnapshot, options: { persist?: boolean } = {}) {
		this.applySnapshot(snapshot, {
			apply: typeof window !== 'undefined',
			persist: options.persist ?? false,
		});
		this.setupSystemPreferenceDetection({ preserveInitialValues: true });
		this._initialized = true;
	}

	setColorScheme(scheme: ColorScheme) {
		this.ensureInitialized();
		this._preferences.colorScheme = scheme;
		this.saveAndApply();
	}

	setDensity(density: Density) {
		this.ensureInitialized();
		this._preferences.density = density;
		this.saveAndApply();
	}

	setFontSize(size: FontSize) {
		this.ensureInitialized();
		this._preferences.fontSize = size;
		this.saveAndApply();
	}

	setFontScale(scale: number) {
		this.ensureInitialized();
		this._preferences.fontScale = Math.max(0.85, Math.min(1.5, scale));
		this.saveAndApply();
	}

	setMotion(motion: MotionPreference) {
		this.ensureInitialized();
		this._preferences.motion = motion;
		this.saveAndApply();
	}

	setCustomColors(colors: Partial<ThemeColors>) {
		this.ensureInitialized();
		this._preferences.customColors = {
			...this._preferences.customColors,
			...colors,
		};
		this.saveAndApply();
	}

	setHighContrastMode(enabled: boolean) {
		this.ensureInitialized();
		this._preferences.highContrastMode = enabled;
		this.saveAndApply();
	}

	updatePreferences(updates: Partial<UserPreferences>) {
		this.ensureInitialized();
		this._preferences = mergePreferences(this._preferences, updates);
		this.saveAndApply();
	}

	reset() {
		this.ensureInitialized();
		this._preferences = clonePreferences(DEFAULT_PREFERENCES);
		this.saveAndApply();
	}

	export(): string {
		this.ensureInitialized();
		return JSON.stringify(this._preferences, null, 2);
	}

	import(json: string): boolean {
		this.ensureInitialized();

		try {
			const imported = JSON.parse(json) as Partial<UserPreferences>;
			if (validatePreferences(imported)) {
				this._preferences = mergePreferences(DEFAULT_PREFERENCES, imported);
				this.saveAndApply();
				return true;
			}
			return false;
		} catch {
			return false;
		}
	}

	private loadPreferences() {
		if (typeof window === 'undefined') {
			return;
		}

		try {
			const stored = localStorage.getItem(PREFERENCES_KEY);
			if (!stored) {
				return;
			}

			const parsed = JSON.parse(stored) as Partial<UserPreferences>;
			if (validatePreferences(parsed)) {
				this._preferences = mergePreferences(DEFAULT_PREFERENCES, parsed);
			}
		} catch (error) {
			console.warn('Failed to load preferences from localStorage:', error);
		}
	}

	private savePreferences() {
		if (typeof window === 'undefined') {
			return;
		}

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

	private setupSystemPreferenceDetection(options: { preserveInitialValues?: boolean } = {}) {
		if (typeof window === 'undefined' || this._systemPreferenceDetectionReady) {
			return;
		}

		const preserveInitialValues = options.preserveInitialValues ?? false;

		this.darkModeQuery = window.matchMedia('(prefers-color-scheme: dark)');
		if (!preserveInitialValues) {
			this._systemColorScheme = this.darkModeQuery.matches ? 'dark' : 'light';
		}
		this.darkModeQuery.addEventListener('change', (event) => {
			this._systemColorScheme = event.matches ? 'dark' : 'light';
			if (this._preferences.colorScheme === 'auto') {
				this.applyTheme();
			}
		});

		this.reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		if (!preserveInitialValues) {
			this._systemMotion = this.reducedMotionQuery.matches ? 'reduced' : 'normal';
		}
		this.reducedMotionQuery.addEventListener('change', (event) => {
			this._systemMotion = event.matches ? 'reduced' : 'normal';
			this.applyTheme();
		});

		this.highContrastQuery = window.matchMedia('(prefers-contrast: high)');
		if (!preserveInitialValues) {
			this._systemHighContrast = this.highContrastQuery.matches;
		}
		this.highContrastQuery.addEventListener('change', (event) => {
			this._systemHighContrast = event.matches;
			this.applyTheme();
		});

		this._systemPreferenceDetectionReady = true;
	}

	private applyTheme() {
		if (typeof window === 'undefined') {
			return;
		}

		applyThemeDocumentAttributes(
			{
				preferences: this._preferences,
				systemColorScheme: this._systemColorScheme,
				systemMotion: this._systemMotion,
				systemHighContrast: this._systemHighContrast,
			},
			document.documentElement
		);
	}

	destroy() {
		if (this.darkModeQuery) {
			// Event listener references are attached inline and intentionally left in place.
		}
	}
}

export const preferencesStore = new PreferencesStore();

export function getPreferences() {
	return preferencesStore.preferences;
}

export function getPreferenceState() {
	return preferencesStore.state;
}
