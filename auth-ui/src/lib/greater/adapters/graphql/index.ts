/**
 * GraphQL Adapter Exports
 *
 * Exports all GraphQL-related functionality for Lesser integration
 */

export { createGraphQLClient, getGraphQLClient, closeGraphQLClient } from './client.js';
export type { GraphQLClientConfig, GraphQLClientInstance } from './client.js';

export { LesserGraphQLAdapter, createLesserGraphQLAdapter } from './LesserGraphQLAdapter.js';
export type {
	LesserGraphQLAdapterConfig,
	TimelineVariables,
	CreateNoteVariables,
	SearchVariables,
	ViewerQuery,
} from './LesserGraphQLAdapter.js';

export * from './generated/types.js';
export { typePolicies, cacheConfig, evictStaleCache, limitCacheSize } from './cache.js';

export * as optimistic from './optimistic.js';
