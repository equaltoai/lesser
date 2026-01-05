/**
 * WCAG Contrast Utilities
 */
/**
 * Calculate contrast ratio between two colors
 */
export declare function getContrastRatio(color1: string, color2: string): number;
/**
 * Check if contrast meets WCAG standards
 */
export declare function meetsWCAG(
	color1: string,
	color2: string,
	level?: 'AA' | 'AAA',
	fontSize?: 'small' | 'large'
): boolean;
/**
 * Suggest a text color (black or white) for a given background
 */
export declare function suggestTextColor(backgroundColor: string): '#000000' | '#FFFFFF';
//# sourceMappingURL=contrast.d.ts.map
