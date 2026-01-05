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
export declare function debounce<T extends (...args: unknown[]) => unknown>(
	fn: T,
	wait: number
): (...args: Parameters<T>) => void;
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
export declare function throttle<T extends (...args: unknown[]) => unknown>(
	fn: T,
	limit: number,
	options?: {
		leading?: boolean;
		trailing?: boolean;
	}
): (...args: Parameters<T>) => void;
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
export declare function rafThrottle<T extends (...args: unknown[]) => unknown>(
	fn: T
): (...args: Parameters<T>) => void;
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
export declare function debounceImmediate<T extends (...args: unknown[]) => unknown>(
	fn: T,
	wait: number,
	immediate?: boolean
): ((...args: Parameters<T>) => void) & {
	cancel: () => void;
};
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
export declare function memoize<TArgs extends unknown[], TResult>(
	fn: (...args: TArgs) => TResult,
	keyResolver?: (...args: TArgs) => string
): ((...args: TArgs) => TResult) & {
	cache: Map<string, TResult>;
};
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
export declare function batch<T>(fn: (items: T[]) => void, wait: number): (item: T) => void;
/**
 * Lazy load a module
 *
 * @param loader Module loader function
 * @returns Promise that resolves to the module
 *
 * @example
 * const HeavyComponent = lazy(() => import('./HeavyComponent.svelte'));
 */
export declare function lazy<T>(loader: () => Promise<T>): () => Promise<T>;
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
export declare function createLRUCache<K, V>(
	maxSize: number
): {
	get: (key: K) => V | undefined;
	set: (key: K, value: V) => void;
	has: (key: K) => boolean;
	delete: (key: K) => boolean;
	clear: () => void;
	size: () => number;
};
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
export declare function measureTime<TArgs extends unknown[], TResult>(
	fn: (...args: TArgs) => TResult,
	label?: string,
	logger?: (message: string) => void
): (...args: TArgs) => TResult;
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
export declare function createResourcePool<T>(
	factory: () => T,
	maxSize: number
): {
	acquire: () => T;
	release: (resource: T) => void;
	size: () => number;
	drain: () => void;
};
//# sourceMappingURL=performance.d.ts.map
