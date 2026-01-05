/**
 * Apollo Client Cache Configuration for Lesser
 *
 * Configures normalized caching with:
 * - Type policies for pagination
 * - Field policies for merging
 * - Cache key generation
 * - Garbage collection
 */
import type { ApolloCache, InMemoryCacheConfig, TypePolicies } from '@apollo/client';
/**
 * Type policies for Apollo Client cache
 */
export declare const typePolicies: TypePolicies;
/**
 * Apollo Client cache configuration
 */
export declare const cacheConfig: InMemoryCacheConfig;
/**
 * Cache eviction policies
 */
export declare const cacheEvictionPolicies: {
	/**
	 * Maximum age for cached data (in milliseconds)
	 */
	maxAge: {
		default: number;
		timeline: number;
		notifications: number;
		search: number;
	};
	/**
	 * Maximum number of items to cache per query
	 */
	maxItems: {
		timeline: number;
		notifications: number;
		search: number;
		conversations: number;
	};
};
/**
 * Helper to evict stale cache entries
 */
export declare function evictStaleCache(
	cache: ApolloCache,
	fieldName: string,
	maxAge: number
): void;
/**
 * Helper to limit cache size
 */
export declare function limitCacheSize(
	cache: ApolloCache,
	fieldName: string,
	maxItems: number
): void;
//# sourceMappingURL=cache.d.ts.map
