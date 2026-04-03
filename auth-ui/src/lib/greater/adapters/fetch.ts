export type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export function resolveFetchLike<T extends FetchLike>(customFetch?: T): T {
	if (customFetch) {
		return customFetch;
	}

	if (typeof globalThis.fetch !== 'function') {
		throw new Error('Global fetch is not available');
	}

	const defaultFetch = globalThis.fetch;
	return ((input: RequestInfo | URL, init?: RequestInit) =>
		defaultFetch.call(globalThis, input, init)) as T;
}
