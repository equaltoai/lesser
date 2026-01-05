/**
 * Generate a stable DOM id that is SSR/hydration-safe when used with `IdProvider`.
 *
 * - With `IdProvider`: deterministic ids (counter-based) during SSR + hydration
 * - Without `IdProvider`: id is assigned onMount (avoids hydration mismatches)
 */
export declare function useStableId(localPrefix?: string): {
	value: string;
};
//# sourceMappingURL=use-stable-id.svelte.d.ts.map
