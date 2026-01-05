/**
 * Color Harmony and Conversion Utilities
 */

export interface HSL {
	h: number; // 0-360
	s: number; // 0-100
	l: number; // 0-100
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
export function hexToHsl(hex: string): HSL {
	let r = 0,
		g = 0,
		b = 0;
	if (hex.length === 4) {
		r = parseInt('0x' + hex[1] + hex[1]);
		g = parseInt('0x' + hex[2] + hex[2]);
		b = parseInt('0x' + hex[3] + hex[3]);
	} else if (hex.length === 7) {
		r = parseInt('0x' + hex[1] + hex[2]);
		g = parseInt('0x' + hex[3] + hex[4]);
		b = parseInt('0x' + hex[5] + hex[6]);
	}

	r /= 255;
	g /= 255;
	b /= 255;

	const cmin = Math.min(r, g, b),
		cmax = Math.max(r, g, b),
		delta = cmax - cmin;

	let h = 0,
		s = 0,
		l = 0;

	if (delta === 0) h = 0;
	else if (cmax === r) h = ((g - b) / delta) % 6;
	else if (cmax === g) h = (b - r) / delta + 2;
	else h = (r - g) / delta + 4;

	h = Math.round(h * 60);

	if (h < 0) h += 360;

	l = (cmax + cmin) / 2;
	s = delta === 0 ? 0 : delta / (1 - Math.abs(2 * l - 1));

	s = +(s * 100).toFixed(1);
	l = +(l * 100).toFixed(1);

	return { h, s, l };
}

/**
 * Convert HSL to Hex
 */
export function hslToHex({ h, s, l }: HSL): string {
	s /= 100;
	l /= 100;

	const c = (1 - Math.abs(2 * l - 1)) * s;
	const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
	const m = l - c / 2;
	let r = 0,
		g = 0,
		b = 0;

	if (0 <= h && h < 60) {
		r = c;
		g = x;
		b = 0;
	} else if (60 <= h && h < 120) {
		r = x;
		g = c;
		b = 0;
	} else if (120 <= h && h < 180) {
		r = 0;
		g = c;
		b = x;
	} else if (180 <= h && h < 240) {
		r = 0;
		g = x;
		b = c;
	} else if (240 <= h && h < 300) {
		r = x;
		g = 0;
		b = c;
	} else if (300 <= h && h < 360) {
		r = c;
		g = 0;
		b = x;
	}

	r = Math.round((r + m) * 255);
	g = Math.round((g + m) * 255);
	b = Math.round((b + m) * 255);

	const toHex = (n: number) => {
		const hex = n.toString(16);
		return hex.length === 1 ? '0' + hex : hex;
	};

	return '#' + toHex(r) + toHex(g) + toHex(b);
}

/**
 * Generate color harmonies from a seed color
 */
export function generateColorHarmony(seedColor: string): ColorHarmony {
	const base = hexToHsl(seedColor);

	const shiftHue = (hsl: HSL, degrees: number): string => {
		let newH = (hsl.h + degrees) % 360;
		if (newH < 0) newH += 360;
		return hslToHex({ ...hsl, h: newH });
	};

	return {
		base: seedColor,
		complementary: [shiftHue(base, 180)],
		analogous: [shiftHue(base, -30), shiftHue(base, 30)],
		triadic: [shiftHue(base, 120), shiftHue(base, 240)],
		tetradic: [shiftHue(base, 90), shiftHue(base, 180), shiftHue(base, 270)],
		splitComplementary: [shiftHue(base, 150), shiftHue(base, 210)],
		monochromatic: [
			hslToHex({ ...base, l: Math.max(0, base.l - 20) }),
			hslToHex({ ...base, l: Math.min(100, base.l + 20) }),
		],
	};
}
