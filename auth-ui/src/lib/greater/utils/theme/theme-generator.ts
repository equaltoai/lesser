import { hexToHsl, hslToHex } from './color-harmony.js';

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

function generateScale(hex: string): ColorScale {
	const hsl = hexToHsl(hex);

	// This is a simplified scale generation logic
	const scale: Partial<ColorScale> = {};

	const lightnessMap = {
		50: 95,
		100: 90,
		200: 80,
		300: 70,
		400: 60,
		500: 50, // Base usually aligns here, but we keep base Hue/Sat
		600: 40,
		700: 30,
		800: 20,
		900: 10,
	};

	// We adjust saturation slightly for lighter/darker shades to look more natural
	for (const [key, l] of Object.entries(lightnessMap)) {
		// Adjust saturation: lower saturation for very light/dark colors usually looks better
		// but let's keep it simple for now and just use the base saturation
		scale[key as unknown as keyof ColorScale] = hslToHex({ h: hsl.h, s: hsl.s, l });
	}

	// Ensure the base color is represented accurately if it's close to a step?
	// For this generator, we just generate a fresh scale based on the HUE/SAT of the input.

	return scale as ColorScale;
}

/**
 * Generate a basic theme from a primary color
 */
export function generateTheme(primaryColor: string): ThemeTokens {
	const primaryScale = generateScale(primaryColor);

	// Generate a secondary color (complementary or split comp)
	const primaryHsl = hexToHsl(primaryColor);
	const secondaryHsl = { ...primaryHsl, h: (primaryHsl.h + 180) % 360 };
	const secondaryScale = generateScale(hslToHex(secondaryHsl));

	// Neutral scale (very low saturation version of primary)
	const neutralHsl = { ...primaryHsl, s: 5 }; // 5% saturation
	const neutralScale = generateScale(hslToHex(neutralHsl));

	return {
		colors: {
			primary: primaryScale,
			secondary: secondaryScale,
			neutral: neutralScale,
			background: '#ffffff',
			surface: '#f8f9fa',
			text: '#1a1a1a',
		},
	};
}
