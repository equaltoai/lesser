/**
 * Color Harmony and Conversion Utilities
 */
export interface HSL {
	h: number;
	s: number;
	l: number;
}
export interface ColorHarmony {
	base: string;
	complementary: string[];
	analogous: string[];
	triadic: string[];
	tetradic: string[];
	splitComplementary: string[];
	monochromatic: string[];
}
/**
 * Convert Hex color to HSL
 */
export declare function hexToHsl(hex: string): HSL;
/**
 * Convert HSL to Hex
 */
export declare function hslToHex({ h, s, l }: HSL): string;
/**
 * Generate color harmonies from a seed color
 */
export declare function generateColorHarmony(seedColor: string): ColorHarmony;
//# sourceMappingURL=color-harmony.d.ts.map
