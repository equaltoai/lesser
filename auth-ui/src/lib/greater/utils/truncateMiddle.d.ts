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
export declare function truncateMiddle(value: string, opts?: TruncateMiddleOptions): string;
//# sourceMappingURL=truncateMiddle.d.ts.map
