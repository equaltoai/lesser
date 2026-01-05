/**
 * WCAG Accessibility Utilities for Greater Components Design Tokens
 * Provides contrast ratio calculation and WCAG AA/AAA compliance verification
 */

/**
 * WCAG 2.1 contrast ratio requirements
 */
export const WCAG_REQUIREMENTS = {
	AA: {
		normalText: 4.5,
		largeText: 3.0,
		uiComponents: 3.0,
	},
	AAA: {
		normalText: 7.0,
		largeText: 4.5,
	},
} as const;

/**
 * Parses a hex color string to RGB values
 */
export function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
	const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
	if (!result) return null;

	const r = result[1];
	const g = result[2];
	const b = result[3];

	if (!r || !g || !b) return null;

	return {
		r: parseInt(r, 16),
		g: parseInt(g, 16),
		b: parseInt(b, 16),
	};
}

/**
 * Calculates relative luminance per WCAG 2.1 specification
 * @see https://www.w3.org/WAI/GL/wiki/Relative_luminance
 */
export function getRelativeLuminance(r: number, g: number, b: number): number {
	const [rs, gs, bs] = [r, g, b].map((c) => {
		const sRGB = c / 255;
		return sRGB <= 0.03928 ? sRGB / 12.92 : Math.pow((sRGB + 0.055) / 1.055, 2.4);
	}) as [number, number, number];
	return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs;
}

/**
 * Calculates contrast ratio between two colors per WCAG 2.1
 * @see https://www.w3.org/WAI/GL/wiki/Contrast_ratio
 */
export function getContrastRatio(color1: string, color2: string): number {
	const rgb1 = hexToRgb(color1);
	const rgb2 = hexToRgb(color2);

	if (!rgb1 || !rgb2) {
		throw new Error(`Invalid hex color: ${!rgb1 ? color1 : color2}`);
	}

	const l1 = getRelativeLuminance(rgb1.r, rgb1.g, rgb1.b);
	const l2 = getRelativeLuminance(rgb2.r, rgb2.g, rgb2.b);

	const lighter = Math.max(l1, l2);
	const darker = Math.min(l1, l2);

	return (lighter + 0.05) / (darker + 0.05);
}

/**
 * Checks if a color combination meets WCAG AA requirements
 */
export function meetsWCAG_AA(
	foreground: string,
	background: string,
	isLargeText: boolean = false
): boolean {
	const ratio = getContrastRatio(foreground, background);
	const requirement = isLargeText
		? WCAG_REQUIREMENTS.AA.largeText
		: WCAG_REQUIREMENTS.AA.normalText;
	return ratio >= requirement;
}

/**
 * Checks if a color combination meets WCAG AAA requirements
 */
export function meetsWCAG_AAA(
	foreground: string,
	background: string,
	isLargeText: boolean = false
): boolean {
	const ratio = getContrastRatio(foreground, background);
	const requirement = isLargeText
		? WCAG_REQUIREMENTS.AAA.largeText
		: WCAG_REQUIREMENTS.AAA.normalText;
	return ratio >= requirement;
}

/**
 * Result of a contrast check
 */
export interface ContrastCheckResult {
	foreground: string;
	background: string;
	ratio: number;
	meetsAA: boolean;
	meetsAAA: boolean;
	meetsAALargeText: boolean;
	meetsAAALargeText: boolean;
}

/**
 * Performs a comprehensive contrast check between two colors
 */
export function checkContrast(foreground: string, background: string): ContrastCheckResult {
	const ratio = getContrastRatio(foreground, background);
	return {
		foreground,
		background,
		ratio: Math.round(ratio * 100) / 100,
		meetsAA: ratio >= WCAG_REQUIREMENTS.AA.normalText,
		meetsAAA: ratio >= WCAG_REQUIREMENTS.AAA.normalText,
		meetsAALargeText: ratio >= WCAG_REQUIREMENTS.AA.largeText,
		meetsAAALargeText: ratio >= WCAG_REQUIREMENTS.AAA.largeText,
	};
}

/**
 * Critical color pairs that must meet WCAG AA for each theme
 */
export const CRITICAL_COLOR_PAIRS = [
	{ name: 'Primary text on background', fg: 'foreground.primary', bg: 'background.primary' },
	{ name: 'Secondary text on background', fg: 'foreground.secondary', bg: 'background.primary' },
	{ name: 'Primary action on background', fg: 'action.primary.default', bg: 'background.primary' },
	{
		name: 'Success text on success bg',
		fg: 'action.success.foreground',
		bg: 'action.success.background',
	},
	{
		name: 'Warning text on warning bg',
		fg: 'action.warning.foreground',
		bg: 'action.warning.background',
	},
	{
		name: 'Error text on error bg',
		fg: 'action.danger.foreground',
		bg: 'action.danger.background',
	},
	{ name: 'Link on background', fg: 'action.link.default', bg: 'background.primary' },
	{
		name: 'Disabled text on background',
		fg: 'foreground.disabled',
		bg: 'background.primary',
		isLargeText: true,
	},
] as const;

/**
 * Validates all critical color pairs for a theme
 */
export function validateThemeContrast(
	_themeColors: Record<string, string>,
	getColor: (path: string) => string
): { valid: boolean; results: Array<ContrastCheckResult & { name: string; required: number }> } {
	const results: Array<ContrastCheckResult & { name: string; required: number }> = [];
	let valid = true;

	for (const pair of CRITICAL_COLOR_PAIRS) {
		try {
			const fg = getColor(pair.fg);
			const bg = getColor(pair.bg);
			const result = checkContrast(fg, bg);
			const isLargeText = 'isLargeText' in pair && pair.isLargeText;
			const required = isLargeText
				? WCAG_REQUIREMENTS.AA.largeText
				: WCAG_REQUIREMENTS.AA.normalText;
			const passes = isLargeText ? result.meetsAALargeText : result.meetsAA;

			results.push({
				...result,
				name: pair.name,
				required,
			});

			if (!passes) {
				valid = false;
			}
		} catch {
			// Color not found or invalid, skip
		}
	}

	return { valid, results };
}
