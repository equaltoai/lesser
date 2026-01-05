import { getContext, onMount } from 'svelte';

interface IdContext {
	current: number;
	prefix: string;
}

let fallbackCounter = 0;

function nextFallbackId(localPrefix: string): string {
	fallbackCounter += 1;
	return `gr-${localPrefix}-${fallbackCounter}`;
}

/**
 * Generate a stable DOM id that is SSR/hydration-safe when used with `IdProvider`.
 *
 * - With `IdProvider`: deterministic ids (counter-based) during SSR + hydration
 * - Without `IdProvider`: id is assigned onMount (avoids hydration mismatches)
 */
export function useStableId(localPrefix: string = 'id'): { value: string } {
	const context = getContext<IdContext>('gr-id-provider');

	if (context) {
		context.current += 1;
		return {
			value: `${context.prefix}-${localPrefix}-${context.current}`,
		};
	}

	let id = $state('');

	onMount(() => {
		id = nextFallbackId(localPrefix);
	});

	return {
		get value() {
			return id;
		},
	};
}
