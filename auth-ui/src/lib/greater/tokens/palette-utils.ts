/**
 * Palette utilities for Greater Components theme system
 * Provides types and functions for palette preset selection and custom palette configuration
 */

// Import palettes data directly from JSON
import palettesData from './palettes.json' with { type: 'json' };

/** Available preset palette names */
export type PalettePreset = 'slate' | 'stone' | 'neutral' | 'zinc' | 'gray';

/** Color scale shade values (50-950) */
export type ColorShade =
	| '50'
	| '100'
	| '200'
	| '300'
	| '400'
	| '500'
	| '600'
	| '700'
	| '800'
	| '900'
	| '950';

/** A complete color scale with all shade values */
export type ColorScale = {
	[K in ColorShade]?: string;
};

/** Custom palette configuration object */
export interface CustomPalette {
	/** Gray scale colors (backgrounds, borders, text) */
	gray?: ColorScale;
	/** Primary brand colors */
	primary?: ColorScale;
}

/** Palette configuration - either a preset name or custom palette object */
export type PaletteConfig = PalettePreset | CustomPalette;

/** Palette data structure from palettes.json */
export interface PaletteData {
	description: string;
	gray: {
		[shade: string]: { value: string };
	};
	primary?: {
		[shade: string]: { value: string };
	};
}

/** All available palettes */
export const palettes = palettesData as Record<PalettePreset, PaletteData>;

/**
 * Generates CSS custom property declarations for a color scale
 * @param prefix - The CSS variable prefix (e.g., 'gray', 'primary')
 * @param scale - The color scale object
 * @returns CSS declarations string
 */
export function generateColorScaleCSS(prefix: string, scale: ColorScale): string {
	const declarations: string[] = [];

	for (const [shade, value] of Object.entries(scale)) {
		if (value) {
			declarations.push(`--gr-color-${prefix}-${shade}: ${value};`);
		}
	}

	return declarations.join('\n');
}

/**
 * Generates CSS custom property declarations for semantic action colors
 * derived from the primary palette
 * @param primaryScale - The primary color scale
 * @returns CSS declarations string for semantic action colors
 */
export function generateSemanticActionCSS(primaryScale: ColorScale): string {
	const declarations: string[] = [];

	// Map primary colors to semantic action colors
	if (primaryScale['600']) {
		declarations.push(`--gr-semantic-action-primary-default: ${primaryScale['600']};`);
	}
	if (primaryScale['700']) {
		declarations.push(`--gr-semantic-action-primary-hover: ${primaryScale['700']};`);
	}
	if (primaryScale['800']) {
		declarations.push(`--gr-semantic-action-primary-active: ${primaryScale['800']};`);
	}
	if (primaryScale['300']) {
		declarations.push(`--gr-semantic-action-primary-disabled: ${primaryScale['300']};`);
	}
	if (primaryScale['500']) {
		declarations.push(`--gr-semantic-focus-ring: ${primaryScale['500']};`);
	}

	return declarations.join('\n');
}

/**
 * Generates complete CSS for a custom palette including semantic derivations
 * @param palette - The custom palette configuration
 * @returns Complete CSS string for injection
 */
export function generatePaletteCSS(palette: CustomPalette): string {
	const parts: string[] = [];

	if (palette.gray) {
		parts.push(generateColorScaleCSS('gray', palette.gray));
	}

	if (palette.primary) {
		parts.push(generateColorScaleCSS('primary', palette.primary));
		parts.push(generateSemanticActionCSS(palette.primary));
	}

	return parts.join('\n');
}

/**
 * Converts a palette preset's gray scale to a ColorScale object
 * @param presetName - The name of the preset palette
 * @returns ColorScale object or undefined if preset not found
 */
export function getPresetGrayScale(presetName: PalettePreset): ColorScale | undefined {
	const preset = palettes[presetName];
	if (!preset) return undefined;

	const scale: ColorScale = {};
	for (const [shade, data] of Object.entries(preset.gray)) {
		scale[shade as ColorShade] = data.value;
	}
	return scale;
}

/**
 * Validates a color scale has required shades
 * @param scale - The color scale to validate
 * @param requiredShades - Array of required shade values
 * @returns True if all required shades are present
 */
export function validateColorScale(
	scale: ColorScale,
	requiredShades: ColorShade[] = [
		'50',
		'100',
		'200',
		'300',
		'400',
		'500',
		'600',
		'700',
		'800',
		'900',
		'950',
	]
): boolean {
	return requiredShades.every((shade) => shade in scale && scale[shade] !== undefined);
}
