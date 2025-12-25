export interface RelativeTimeOptions {
	/**
	 * Locale for formatting (e.g., 'en-US', 'fr-FR')
	 */
	locale?: string;
	/**
	 * Whether to use numeric format (e.g., "1 day ago" vs "yesterday")
	 */
	numeric?: 'always' | 'auto';
	/**
	 * Custom now time for testing
	 */
	now?: Date | number;
	/**
	 * Maximum unit to display
	 */
	maxUnit?: 'year' | 'month' | 'week' | 'day' | 'hour' | 'minute' | 'second';
}

interface TimeUnit {
	unit: Intl.RelativeTimeFormatUnit;
	ms: number;
	max?: number;
}

const TIME_UNITS: TimeUnit[] = [
	{ unit: 'year', ms: 31536000000 },
	{ unit: 'month', ms: 2628000000 },
	{ unit: 'week', ms: 604800000 },
	{ unit: 'day', ms: 86400000 },
	{ unit: 'hour', ms: 3600000 },
	{ unit: 'minute', ms: 60000 },
	{ unit: 'second', ms: 1000, max: 60 },
];

/**
 * Format a date as relative time (e.g., "2 hours ago", "just now")
 * @param date - The date to format
 * @param options - Formatting options
 * @returns Formatted relative time string
 */
export function relativeTime(
	date: Date | string | number,
	options: RelativeTimeOptions = {}
): string {
	const { locale = 'en-US', numeric = 'auto', now = new Date(), maxUnit = 'year' } = options;

	const targetDate = date instanceof Date ? date : new Date(date);
	const nowDate = now instanceof Date ? now : new Date(now);

	if (isNaN(targetDate.getTime())) {
		return 'Invalid date';
	}

	const diff = nowDate.getTime() - targetDate.getTime();
	const absDiff = Math.abs(diff);
	const isFuture = diff < 0;

	// Handle "just now" case
	if (absDiff < 5000) {
		return 'just now';
	}

	// Find the appropriate unit
	for (const { unit, ms, max } of TIME_UNITS) {
		// Skip units larger than maxUnit if specified
		if (maxUnit) {
			const unitOrder = ['year', 'month', 'week', 'day', 'hour', 'minute', 'second'];
			const maxIndex = unitOrder.indexOf(maxUnit);
			const currentIndex = unitOrder.indexOf(unit);
			if (maxIndex >= 0 && currentIndex < maxIndex) {
				continue;
			}
		}

		const value = Math.floor(absDiff / ms);

		if (value >= 1 && (!max || value <= max)) {
			try {
				const formatter = new Intl.RelativeTimeFormat(locale, { numeric });
				return formatter.format(isFuture ? value : -value, unit);
			} catch {
				// Fallback for unsupported locales
				const plural = value === 1 ? '' : 's';
				const timeStr = `${value} ${unit}${plural}`;
				return isFuture ? `in ${timeStr}` : `${timeStr} ago`;
			}
		}
	}

	// Fallback to seconds if nothing else matches
	const seconds = Math.floor(absDiff / 1000);
	try {
		const formatter = new Intl.RelativeTimeFormat(locale, { numeric });
		return formatter.format(isFuture ? seconds : -seconds, 'second');
	} catch {
		const plural = seconds === 1 ? '' : 's';
		const timeStr = `${seconds} second${plural}`;
		return isFuture ? `in ${timeStr}` : `${timeStr} ago`;
	}
}

/**
 * Format a date with both absolute and relative time
 * @param date - The date to format
 * @param options - Formatting options
 * @returns Object with absolute and relative time strings
 */
export function formatDateTime(
	date: Date | string | number,
	options: RelativeTimeOptions & {
		dateStyle?: 'full' | 'long' | 'medium' | 'short';
		timeStyle?: 'full' | 'long' | 'medium' | 'short';
	} = {}
): { absolute: string; relative: string; iso: string } {
	const {
		locale = 'en-US',
		dateStyle = 'medium',
		timeStyle = 'short',
		...relativeOptions
	} = options;

	const targetDate = date instanceof Date ? date : new Date(date);

	if (isNaN(targetDate.getTime())) {
		return {
			absolute: 'Invalid date',
			relative: 'Invalid date',
			iso: '',
		};
	}

	const absolute = new Intl.DateTimeFormat(locale, {
		dateStyle,
		timeStyle,
	}).format(targetDate);

	const relative = relativeTime(targetDate, relativeOptions);
	const iso = targetDate.toISOString();

	return { absolute, relative, iso };
}

/**
 * Get a human-readable duration between two dates
 * @param start - Start date
 * @param end - End date
 * @param options - Formatting options
 * @returns Duration string
 */
export function getDuration(
	start: Date | string | number,
	end: Date | string | number,
	options: { locale?: string; units?: number } = {}
): string {
	const { units = 2 } = options;

	const startDate = start instanceof Date ? start : new Date(start);
	const endDate = end instanceof Date ? end : new Date(end);

	if (isNaN(startDate.getTime()) || isNaN(endDate.getTime())) {
		return 'Invalid date';
	}

	let diff = Math.abs(endDate.getTime() - startDate.getTime());
	const parts: string[] = [];

	for (const { unit, ms } of TIME_UNITS) {
		const value = Math.floor(diff / ms);
		if (value > 0 && parts.length < units) {
			const plural = value === 1 ? '' : 's';
			parts.push(`${value} ${unit}${plural}`);
			diff = diff % ms; // Subtract the value we've accounted for
		}
	}

	return parts.length > 0 ? parts.join(', ') : '0 seconds';
}
