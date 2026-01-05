/**
 * GraphQL Client Setup for Lesser
 *
 * Configures Apollo Client with:
 * - HTTP link for queries/mutations
 * - WebSocket link for subscriptions
 * - Normalized caching
 * - Authentication
 * - Error handling
 * - Retry logic
 */
import { ApolloClient } from '@apollo/client';
import { type Client } from 'graphql-ws';
export interface GraphQLClientConfig {
	/**
	 * HTTP endpoint for queries and mutations
	 * @example 'https://api.lesser.social/graphql'
	 */
	httpEndpoint: string;
	/**
	 * WebSocket endpoint for subscriptions
	 * If not provided, WebSocket will NOT be used - subscriptions will not work
	 * @example 'wss://api.lesser.social/graphql'
	 */
	wsEndpoint?: string;
	/**
	 * Authentication token
	 */
	token?: string;
	/**
	 * Enable debug logging
	 */
	debug?: boolean;
	/**
	 * Custom headers for HTTP requests
	 */
	headers?: Record<string, string>;
	/**
	 * Connection timeout in milliseconds
	 * @default 10000
	 */
	connectionTimeout?: number;
	/**
	 * Enable automatic retry on network errors
	 * @default true
	 */
	enableRetry?: boolean;
	/**
	 * Maximum retry attempts
	 * @default 3
	 */
	maxRetries?: number;
}
export interface GraphQLClientInstance {
	client: ApolloClient;
	wsClient: Client | null;
	updateToken: (token: string | null) => void;
	close: () => void;
}
/**
 * Create Apollo Client with Lesser-specific configuration
 */
export declare function createGraphQLClient(config: GraphQLClientConfig): GraphQLClientInstance;
/**
 * Get or create singleton GraphQL client
 */
export declare function getGraphQLClient(config?: GraphQLClientConfig): GraphQLClientInstance;
/**
 * Close and reset singleton client
 */
export declare function closeGraphQLClient(): void;
//# sourceMappingURL=client.d.ts.map
