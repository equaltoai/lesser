export interface ThemeTokens {
	colors: {
		primary: ColorScale;
		secondary: ColorScale;
		neutral: ColorScale;
		background: string;
		surface: string;
		text: string;
	};
}
export interface ColorScale {
	50: string;
	100: string;
	200: string;
	300: string;
	400: string;
	500: string;
	600: string;
	700: string;
	800: string;
	900: string;
}
/**
 * Generate a basic theme from a primary color
 */
export declare function generateTheme(primaryColor: string): ThemeTokens;
//# sourceMappingURL=theme-generator.d.ts.map
