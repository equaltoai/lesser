/**
 * Performance Optimization Utilities
 *
 * Provides debounce, throttle, and other performance optimization helpers
 * for improving runtime performance in Greater Components.
 */

/**
 * Debounce function - delays execution until after wait time has elapsed since last call
 *
 * @param fn Function to debounce
 * @param wait Wait time in milliseconds
 * @returns Debounced function
 *
 * @example
 * const debouncedSearch = debounce((query: string) => {
 *   console.log('Searching for:', query);
 * }, 300);
 *
 * debouncedSearch('hello');
 * debouncedSearch('hello world'); // Only this will execute after 300ms
 */
export function debounce<T extends (...args: unknown[]) => unknown>(
	fn: T,
	wait: number
): (...args: Parameters<T>) => void {
	let timeoutId: ReturnType<typeof setTimeout> | null = null;

	return function debounced(...args: Parameters<T>) {
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		timeoutId = setTimeout(() => {
			fn(...args);
			timeoutId = null;
		}, wait);
	};
}

/**
 * Throttle function - ensures function is only called once per interval
 *
 * @param fn Function to throttle
 * @param limit Time limit in milliseconds
 * @param options Configuration options
 * @returns Throttled function
 *
 * @example
 * const throttledScroll = throttle(() => {
 *   console.log('Scrolling...');
 * }, 100);
 *
 * window.addEventListener('scroll', throttledScroll);
 */
export function throttle<T extends (...args: unknown[]) => unknown>(
	fn: T,
	limit: number,
	options: {
		leading?: boolean;
		trailing?: boolean;
	} = {}
): (...args: Parameters<T>) => void {
	const { leading = true, trailing = true } = options;

	let lastRan: number | null = null;
	let lastFunc: ReturnType<typeof setTimeout> | null = null;

	return function throttled(...args: Parameters<T>) {
		const now = Date.now();

		if (lastRan === null && !leading) {
			lastRan = now;
		}

		const remaining = limit - (now - (lastRan || 0));

		if (remaining <= 0 || remaining > limit) {
			if (lastFunc !== null) {
				clearTimeout(lastFunc);
				lastFunc = null;
			}

			lastRan = now;
			fn(...args);
		} else if (lastFunc === null && trailing) {
			lastFunc = setTimeout(() => {
				lastRan = leading ? Date.now() : null;
				lastFunc = null;
				fn(...args);
			}, remaining);
		}
	};
}

/**
 * Request Animation Frame throttle - ensures function is only called once per frame
 *
 * @param fn Function to throttle
 * @returns RAF-throttled function
 *
 * @example
 * const rafScroll = rafThrottle(() => {
 *   console.log('Frame update');
 * });
 *
 * window.addEventListener('scroll', rafScroll);
 */
export function rafThrottle<T extends (...args: unknown[]) => unknown>(
	fn: T
): (...args: Parameters<T>) => void {
	let rafId: number | null = null;

	return function throttled(...args: Parameters<T>) {
		if (rafId !== null) {
			return;
		}

		rafId = requestAnimationFrame(() => {
			fn(...args);
			rafId = null;
		});
	};
}

/**
 * Debounce with immediate execution option
 *
 * @param fn Function to debounce
 * @param wait Wait time in milliseconds
 * @param immediate Execute immediately on first call
 * @returns Debounced function with cancel method
 *
 * @example
 * const debouncedSave = debounceImmediate((data) => {
 *   console.log('Saving:', data);
 * }, 1000, true);
 *
 * debouncedSave({ foo: 'bar' }); // Executes immediately
 * debouncedSave({ foo: 'baz' }); // Debounced
 */
export function debounceImmediate<T extends (...args: unknown[]) => unknown>(
	fn: T,
	wait: number,
	immediate: boolean = false
): ((...args: Parameters<T>) => void) & { cancel: () => void } {
	let timeoutId: ReturnType<typeof setTimeout> | null = null;

	const debounced = function (...args: Parameters<T>) {
		const callNow = immediate && timeoutId === null;

		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		timeoutId = setTimeout(() => {
			timeoutId = null;
			if (!immediate) {
				fn(...args);
			}
		}, wait);

		if (callNow) {
			fn(...args);
		}
	};

	debounced.cancel = () => {
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
			timeoutId = null;
		}
	};

	return debounced;
}

/**
 * Memoize function results
 *
 * @param fn Function to memoize
 * @param keyResolver Custom key resolver function
 * @returns Memoized function with cache
 *
 * @example
 * const expensiveCalc = memoize((n: number) => {
 *   return n * n;
 * });
 *
 * expensiveCalc(5); // Calculates
 * expensiveCalc(5); // Returns cached result
 */
export function memoize<TArgs extends unknown[], TResult>(
	fn: (...args: TArgs) => TResult,
	keyResolver?: (...args: TArgs) => string
): ((...args: TArgs) => TResult) & { cache: Map<string, TResult> } {
	const cache = new Map<string, TResult>();

	const memoized = function (...args: TArgs): TResult {
		const key = keyResolver ? keyResolver(...args) : JSON.stringify(args);

		if (cache.has(key)) {
			const cached = cache.get(key);
			return cached as TResult;
		}

		const result = fn(...args);
		cache.set(key, result);
		return result;
	};

	return Object.assign(memoized, { cache }) as ((...args: TArgs) => TResult) & {
		cache: Map<string, TResult>;
	};
}

/**
 * Batch function calls and execute once
 *
 * @param fn Function to batch
 * @param wait Wait time before executing batch
 * @returns Batched function
 *
 * @example
 * const batchedUpdate = batch((items: string[]) => {
 *   console.log('Updating items:', items);
 * }, 100);
 *
 * batchedUpdate('item1');
 * batchedUpdate('item2');
 * batchedUpdate('item3');
 * // Executes once with ['item1', 'item2', 'item3']
 */
export function batch<T>(fn: (items: T[]) => void, wait: number): (item: T) => void {
	let items: T[] = [];
	let timeoutId: ReturnType<typeof setTimeout> | null = null;

	return function batched(item: T) {
		items.push(item);

		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		timeoutId = setTimeout(() => {
			fn(items);
			items = [];
			timeoutId = null;
		}, wait);
	};
}

/**
 * Lazy load a module
 *
 * @param loader Module loader function
 * @returns Promise that resolves to the module
 *
 * @example
 * const HeavyComponent = lazy(() => import('./HeavyComponent.svelte'));
 */
export function lazy<T>(loader: () => Promise<T>): () => Promise<T> {
	let cachedModule: T | null = null;
	let loading: Promise<T> | null = null;

	return async () => {
		if (cachedModule !== null) {
			return cachedModule;
		}

		if (loading !== null) {
			return loading;
		}

		loading = loader().then((module) => {
			cachedModule = module;
			loading = null;
			return module;
		});

		return loading;
	};
}

/**
 * Create an LRU (Least Recently Used) cache
 *
 * @param maxSize Maximum number of items to cache
 * @returns LRU cache instance
 *
 * @example
 * const cache = createLRUCache<string, number>(100);
 * cache.set('key1', 123);
 * cache.get('key1'); // 123
 */
export function createLRUCache<K, V>(
	maxSize: number
): {
	get: (key: K) => V | undefined;
	set: (key: K, value: V) => void;
	has: (key: K) => boolean;
	delete: (key: K) => boolean;
	clear: () => void;
	size: () => number;
} {
	const cache = new Map<K, V>();

	return {
		get(key: K): V | undefined {
			if (!cache.has(key)) {
				return undefined;
			}

			// Move to end (most recently used)
			const value = cache.get(key) as V;
			cache.delete(key);
			cache.set(key, value);

			return value;
		},

		set(key: K, value: V): void {
			// Delete if exists to reinsert at end
			if (cache.has(key)) {
				cache.delete(key);
			}

			cache.set(key, value);

			// Evict oldest if over size
			if (cache.size > maxSize) {
				const firstKey = cache.keys().next().value;
				if (firstKey !== undefined) {
					cache.delete(firstKey);
				}
			}
		},

		has(key: K): boolean {
			return cache.has(key);
		},

		delete(key: K): boolean {
			return cache.delete(key);
		},

		clear(): void {
			cache.clear();
		},

		size(): number {
			return cache.size;
		},
	};
}

/**
 * Measure execution time of a function
 *
 * @param fn Function to measure
 * @param label Optional label for logging
 * @returns Wrapped function that logs execution time
 *
 * @example
 * const measured = measureTime(() => {
 *   // expensive operation
 * }, 'Heavy Calculation');
 */
export function measureTime<TArgs extends unknown[], TResult>(
	fn: (...args: TArgs) => TResult,
	label?: string,
	logger: (message: string) => void = console.warn
): (...args: TArgs) => TResult {
	return function measured(...args: TArgs): TResult {
		const start = performance.now();
		const result = fn(...args);
		const end = performance.now();

		const formatted = `${(end - start).toFixed(2)}ms`;
		if (label) {
			logger(`${label}: ${formatted}`);
		} else {
			logger(`Execution time: ${formatted}`);
		}

		return result;
	};
}

/**
 * Create a resource pool for reusing objects
 *
 * @param factory Factory function to create new resources
 * @param maxSize Maximum pool size
 * @returns Resource pool
 *
 * @example
 * const pool = createResourcePool(() => new Worker('worker.js'), 4);
 * const worker = pool.acquire();
 * // Use worker
 * pool.release(worker);
 */
export function createResourcePool<T>(
	factory: () => T,
	maxSize: number
): {
	acquire: () => T;
	release: (resource: T) => void;
	size: () => number;
	drain: () => void;
} {
	const available: T[] = [];
	let created = 0;

	return {
		acquire(): T {
			if (available.length > 0) {
				const resource = available.pop();
				if (resource !== undefined) {
					return resource;
				}
			}

			if (created < maxSize) {
				created++;
				return factory();
			}

			throw new Error('Resource pool exhausted');
		},

		release(resource: T): void {
			if (available.length < maxSize) {
				available.push(resource);
			}
		},

		size(): number {
			return available.length;
		},

		drain(): void {
			available.length = 0;
			created = 0;
		},
	};
}
