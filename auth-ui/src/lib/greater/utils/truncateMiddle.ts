/**
 * Truncates a string by replacing the middle with an ellipsis.
 *
 * Useful for displaying long identifiers like addresses, hashes, or JWT-like IDs.
 *
 * @example
 * truncateMiddle('0x1234567890abcdef'); // "0x1234…cdef"
 *
 * @public
 */
export interface TruncateMiddleOptions {
	/** Number of characters to keep at the start. */
	head?: number;
	/** Number of characters to keep at the end. */
	tail?: number;
	/** String to insert between head and tail (defaults to the ellipsis character). */
	ellipsis?: string;
}

/**
 * Truncate a string in the middle.
 *
 * @public
 */
export function truncateMiddle(value: string, opts: TruncateMiddleOptions = {}): string {
	const { head = 6, tail = 4, ellipsis = '…' } = opts;

	const safeHead = Math.max(0, Math.floor(head));
	const safeTail = Math.max(0, Math.floor(tail));
	const safeEllipsis = ellipsis ?? '…';

	if (value.length === 0) return value;

	if (value.length <= safeHead + safeTail + safeEllipsis.length) return value;

	const start = safeHead > 0 ? value.slice(0, safeHead) : '';
	const end = safeTail > 0 ? value.slice(-safeTail) : '';

	return `${start}${safeEllipsis}${end}`;
}

