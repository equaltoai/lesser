/**
 * Palette utilities for Greater Components theme system
 * Provides types and functions for palette preset selection and custom palette configuration
 */
/** Available preset palette names */
export type PalettePreset = 'slate' | 'stone' | 'neutral' | 'zinc' | 'gray';
/** Available font preset names */
export type FontPreset = 'system' | 'sans' | 'serif' | 'mono';
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
		[shade: string]: {
			value: string;
		};
	};
	primary?: {
		[shade: string]: {
			value: string;
		};
	};
}
/** All available palettes */
export declare const palettes: Record<PalettePreset, PaletteData>;
/**
 * Generates CSS custom property declarations for a color scale
 * @param prefix - The CSS variable prefix (e.g., 'gray', 'primary')
 * @param scale - The color scale object
 * @returns CSS declarations string
 */
export declare function generateColorScaleCSS(prefix: string, scale: ColorScale): string;
/**
 * Generates CSS custom property declarations for semantic action colors
 * derived from the primary palette
 * @param primaryScale - The primary color scale
 * @returns CSS declarations string for semantic action colors
 */
export declare function generateSemanticActionCSS(primaryScale: ColorScale): string;
/**
 * Generates complete CSS for a custom palette including semantic derivations
 * @param palette - The custom palette configuration
 * @returns Complete CSS string for injection
 */
export declare function generatePaletteCSS(palette: CustomPalette): string;
/**
 * Converts a palette preset's gray scale to a ColorScale object
 * @param presetName - The name of the preset palette
 * @returns ColorScale object or undefined if preset not found
 */
export declare function getPresetGrayScale(presetName: PalettePreset): ColorScale | undefined;
/**
 * Validates a color scale has required shades
 * @param scale - The color scale to validate
 * @param requiredShades - Array of required shade values
 * @returns True if all required shades are present
 */
export declare function validateColorScale(
	scale: ColorScale,
	requiredShades?: ColorShade[]
): boolean;
//# sourceMappingURL=palette-utils.d.ts.map
