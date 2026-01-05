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

import {
	ApolloClient,
	InMemoryCache,
	HttpLink,
	split,
	from,
	type DefaultOptions,
} from '@apollo/client';
import { GraphQLWsLink } from '@apollo/client/link/subscriptions/index.js';
import { getMainDefinition } from '@apollo/client/utilities/index.js';
import { onError } from '@apollo/client/link/error/index.js';
import { RetryLink } from '@apollo/client/link/retry/index.js';
import { createClient, type Client } from 'graphql-ws';
import { CombinedGraphQLErrors } from '@apollo/client/errors/index.js';
import { cacheConfig } from './cache.js';

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
	wsClient: Client | null; // null if wsEndpoint not provided
	updateToken: (token: string | null) => void;
	close: () => void;
}

/**
 * Create Apollo Client with Lesser-specific configuration
 */
export function createGraphQLClient(config: GraphQLClientConfig): GraphQLClientInstance {
	const {
		httpEndpoint,
		wsEndpoint: configWsEndpoint,
		token,
		debug = false,
		headers = {},
		connectionTimeout = 10000,
		enableRetry = true,
		maxRetries = 3,
	} = config;

	let currentToken = token || null;

	const logDebug = (message: string, context?: unknown): void => {
		if (!debug) {
			return;
		}
		const output = globalThis.console;
		if (!output?.warn) {
			return;
		}
		if (context !== undefined) {
			output.warn(message, context);
		} else {
			output.warn(message);
		}
	};

	const logDebugError = (message: string, context?: unknown): void => {
		if (!debug) {
			return;
		}
		const output = globalThis.console;
		if (!output?.error) {
			return;
		}
		if (context !== undefined) {
			output.error(message, context);
		} else {
			output.error(message);
		}
	};

	// HTTP Link for queries and mutations
	const httpLink = new HttpLink({
		uri: httpEndpoint,
		headers: {
			...headers,
			...(currentToken && { authorization: `Bearer ${currentToken}` }),
		},
		fetch,
	});

	// WebSocket client for subscriptions
	// CRITICAL: Only create WebSocket code if wsEndpoint is explicitly provided
	// NO FALLBACKS - if wsEndpoint is missing/invalid, skip WebSocket entirely
	let wsClient: Client | null = null;
	let wsLink: GraphQLWsLink | null = null;
	let wsUrl: string | null = null;
	let baseWsEndpoint: string | null = null; // Store original endpoint without token for token updates

	if (configWsEndpoint) {
		// Validate wsEndpoint
		if (typeof configWsEndpoint !== 'string') {
			logDebugError(
				`[GraphQL] Invalid wsEndpoint type: ${typeof configWsEndpoint}. Skipping WebSocket.`
			);
		} else if (configWsEndpoint.trim() === '') {
			logDebugError('[GraphQL] wsEndpoint is empty. Skipping WebSocket.');
		} else {
			const wsEndpoint = configWsEndpoint.trim();

			// Validate wsEndpoint is a valid WebSocket URL
			if (!wsEndpoint.startsWith('ws://') && !wsEndpoint.startsWith('wss://')) {
				logDebugError(
					`[GraphQL] wsEndpoint must start with 'ws://' or 'wss://', got: ${wsEndpoint}. Skipping WebSocket.`
				);
			} else {
				// Ensure wsEndpoint is NOT derived from httpEndpoint - skip if it appears to be
				const derivedFromHttp =
					wsEndpoint === httpEndpoint ||
					wsEndpoint === httpEndpoint.replace('https://', 'wss://') ||
					wsEndpoint === httpEndpoint.replace('http://', 'ws://') ||
					wsEndpoint === httpEndpoint.replace(/^https?:\/\//, 'wss://');

				if (derivedFromHttp) {
					logDebugError(
						`[GraphQL] wsEndpoint appears to be derived from httpEndpoint! ` +
							`httpEndpoint: ${httpEndpoint}, wsEndpoint: ${wsEndpoint}. ` +
							'Skipping WebSocket - wsEndpoint must be explicitly provided.'
					);
				} else {
					// wsEndpoint is valid - create WebSocket client
					// Store the base endpoint (without token) for token updates
					baseWsEndpoint = wsEndpoint;

					// IMPORTANT: do not initialize WebSocket transport until a token is available
					if (!currentToken) {
						throw new Error(
							'[GraphQL] Cannot initialize WebSocket subscriptions without an authentication token.'
						);
					}

					// IMPORTANT: Add token to query string (server requires token in URL)
					let finalWsUrl = String(wsEndpoint);

					// Log token status for debugging
					logDebug(`[GraphQL] Token available for WebSocket: ${currentToken.substring(0, 10)}...`);

					try {
						const url = new URL(finalWsUrl);
						// Use 'access_token' as query param name (can be 'token' if server prefers that)
						url.searchParams.set('access_token', currentToken);
						finalWsUrl = url.toString();
						logDebug(
							`[GraphQL] Added token to WebSocket URL query string: ${finalWsUrl.replace(currentToken, '***REDACTED***')}`
						);
					} catch (error) {
						logDebugError(`[GraphQL] Failed to add token to WebSocket URL: ${error}`);
						// Fallback: manually append if URL parsing fails
						const separator = finalWsUrl.includes('?') ? '&' : '?';
						finalWsUrl = `${finalWsUrl}${separator}access_token=${encodeURIComponent(currentToken)}`;
						logDebug(
							`[GraphQL] Manually added token to WebSocket URL: ${finalWsUrl.replace(currentToken, '***REDACTED***')}`
						);
					}

					wsUrl = finalWsUrl;
					logDebug(
						`[GraphQL] Creating WebSocket client with EXPLICIT endpoint: ${wsUrl.replace(currentToken || '', '***REDACTED***')}`
					);

					// CRITICAL: graphql-ws strips query parameters from URLs
					// We need to use a custom WebSocket implementation that preserves the full URL
					// Create a WebSocket factory that uses the native WebSocket API with the full URL
					const webSocketImpl = class PreserveQueryParamsWebSocket extends WebSocket {
						constructor(url: string | URL, protocols?: string | string[]) {
							// Always use the full URL with query parameters
							super(url, protocols);
							logDebug(
								`[GraphQL] Native WebSocket created with URL: ${String(url).replace(currentToken || '', '***REDACTED***')}`
							);
						}
					};

					// CRITICAL: Log the actual URL that will be used (for debugging)
					logDebug(
						'[GraphQL] DEBUG: WebSocket URL being passed to createClient',
						wsUrl.replace(currentToken || '', '***REDACTED***')
					);
					logDebug('[GraphQL] DEBUG: Has token?', !!currentToken);
					logDebug('[GraphQL] DEBUG: Token length', currentToken?.length ?? 0);

					wsClient = createClient({
						url: wsUrl, // This is the ONLY URL that will be used - no fallbacks
						webSocketImpl: webSocketImpl, // CRITICAL: Use custom WebSocket that preserves query params
						connectionParams: () => {
							// Also include in connectionParams for protocols that support it
							const params: Record<string, string> = {};
							if (currentToken) {
								params['authorization'] = `Bearer ${currentToken}`;
							}
							logDebug(`[GraphQL] WebSocket connectionParams:`, Object.keys(params));
							return params;
						},
						retryAttempts: maxRetries,
						shouldRetry: () => enableRetry,
						connectionAckWaitTimeout: connectionTimeout,
						on: {
							connecting: () => {
								logDebug(
									`[GraphQL] WebSocket connecting to URL: ${wsUrl?.replace(currentToken || '', '***REDACTED***')}`
								);
								// Log the actual URL being used when connecting
								logDebug(
									'[GraphQL] DEBUG: WebSocket connecting - URL',
									wsUrl?.replace(currentToken || '', '***REDACTED***')
								);
							},
							connected: () => {
								logDebug(`[GraphQL] WebSocket connected to EXPLICIT URL: ${wsEndpoint}`);
							},
							closed: () => {
								logDebug('[GraphQL] WebSocket closed');
							},
							error: (error) => {
								logDebugError(
									`[GraphQL] WebSocket error connecting to EXPLICIT URL ${wsEndpoint}`,
									error
								);
								// Log the actual URL being used for debugging
								if (wsUrl) {
									logDebugError(
										`[GraphQL] WebSocket URL used: ${wsUrl.replace(currentToken || '', '***REDACTED***')}`
									);
									console.error(
										'[GraphQL] DEBUG: WebSocket error - URL was:',
										wsUrl.replace(currentToken || '', '***REDACTED***')
									);
								}
							},
						},
					});

					wsLink = new GraphQLWsLink(wsClient);
					logDebug(`[GraphQL] GraphQLWsLink created - WebSocket will ONLY use URL: ${wsUrl}`);
				}
			}
		}
	} else {
		logDebug(
			'[GraphQL] No wsEndpoint provided - WebSocket will NOT be used. Subscriptions will not work.'
		);
	}

	// Error handling link
	const errorLink = onError(({ error }) => {
		if (CombinedGraphQLErrors.is(error)) {
			error.errors.forEach(({ message, locations, path, extensions }) => {
				const errorMsg = `[GraphQL Error]: Message: ${message}, Location: ${JSON.stringify(locations)}, Path: ${path}`;
				logDebugError(errorMsg);

				// Handle authentication errors
				const extensionCode =
					extensions && typeof extensions === 'object'
						? (extensions as Record<string, unknown>)['code']
						: undefined;
				if (extensionCode === 'UNAUTHENTICATED') {
					logDebugError('[GraphQL] Authentication error');
					// Clear token on auth failure
					currentToken = null;
				}
			});
			return;
		}

		logDebugError(`[GraphQL Network Error]: ${error.message}`);
	});

	// Retry link for network errors
	const retryLink = new RetryLink({
		attempts: {
			max: enableRetry ? maxRetries : 0,
			retryIf: (error) => {
				// Retry on network errors but not on GraphQL errors
				return Boolean(error) && !CombinedGraphQLErrors.is(error);
			},
		},
		delay: {
			initial: 300,
			max: 3000,
			jitter: true,
		},
	});

	// Split link: WebSocket for subscriptions (if available), HTTP for queries/mutations
	// If wsLink is null, all operations go through HTTP (subscriptions will fail)
	const splitLink = wsLink
		? split(
				({ query }) => {
					const definition = getMainDefinition(query);
					return (
						definition.kind === 'OperationDefinition' && definition.operation === 'subscription'
					);
				},
				wsLink,
				httpLink
			)
		: httpLink; // No WebSocket - route everything through HTTP

	// Combine all links
	const link = from([errorLink, retryLink, splitLink]);

	// Default options for all operations
	const defaultOptions: DefaultOptions = {
		watchQuery: {
			fetchPolicy: 'cache-and-network',
			errorPolicy: 'all',
		},
		query: {
			fetchPolicy: 'network-only',
			errorPolicy: 'all',
		},
		mutate: {
			errorPolicy: 'all',
		},
	};

	// Create Apollo Client
	const client = new ApolloClient({
		link,
		cache: new InMemoryCache(cacheConfig),
		defaultOptions,
		devtools: {
			enabled: debug,
		},
	});

	// Function to update authentication token
	const updateToken = (newToken: string | null) => {
		currentToken = newToken;

		// Update HTTP link headers
		const newHttpLink = new HttpLink({
			uri: httpEndpoint,
			headers: {
				...headers,
				...(currentToken && { authorization: `Bearer ${currentToken}` }),
			},
			fetch,
		});

		// If token has been cleared, disable WebSocket and fall back to HTTP only
		if (!currentToken) {
			if (wsClient) {
				wsClient.dispose();
			}
			wsClient = null;
			wsUrl = null;
			wsLink = null;

			const newLink = from([errorLink, retryLink, newHttpLink]);
			client.setLink(newLink);
			logDebug('[GraphQL] Token cleared, WebSocket disabled until token is restored');

			void client.clearStore();
			return;
		}

		// Only reconnect WebSocket if it exists
		if (wsClient && baseWsEndpoint) {
			wsClient.dispose();

			// Rebuild URL with new token using base endpoint
			let finalWsUrl: string;
			try {
				const url = new URL(baseWsEndpoint);
				if (currentToken) {
					url.searchParams.set('access_token', currentToken);
				} else {
					// Remove token if logging out
					url.searchParams.delete('access_token');
				}
				finalWsUrl = url.toString();
				logDebug(
					`[GraphQL] Updated WebSocket URL with new token: ${finalWsUrl.replace(currentToken || '', '***REDACTED***')}`
				);
			} catch (error) {
				logDebugError(`[GraphQL] Failed to rebuild WebSocket URL with token: ${error}`);
				// Fallback: manually append if URL parsing fails
				const separator = baseWsEndpoint.includes('?') ? '&' : '?';
				if (currentToken) {
					finalWsUrl = `${baseWsEndpoint}${separator}access_token=${encodeURIComponent(currentToken)}`;
				} else {
					finalWsUrl = baseWsEndpoint;
				}
				logDebug(
					`[GraphQL] Manually rebuilt WebSocket URL: ${finalWsUrl.replace(currentToken || '', '***REDACTED***')}`
				);
			}

			wsUrl = finalWsUrl;

			// CRITICAL: Use custom WebSocket implementation that preserves query parameters
			const webSocketImpl = class PreserveQueryParamsWebSocket extends WebSocket {
				constructor(url: string | URL, protocols?: string | string[]) {
					super(url, protocols);
					logDebug(
						`[GraphQL] Native WebSocket recreated with URL: ${String(url).replace(currentToken || '', '***REDACTED***')}`
					);
				}
			};

			const newWsClient = createClient({
				url: finalWsUrl, // Use URL with token in query string
				webSocketImpl: webSocketImpl, // CRITICAL: Use custom WebSocket that preserves query params
				connectionParams: () => ({
					// Also include in connectionParams for protocols that support it
					...(currentToken && { authorization: `Bearer ${currentToken}` }),
				}),
				retryAttempts: maxRetries,
				shouldRetry: () => enableRetry,
				connectionAckWaitTimeout: connectionTimeout,
				on: {
					connected: () => {
						logDebug(`[GraphQL] WebSocket reconnected to EXPLICIT URL: ${baseWsEndpoint}`);
					},
					closed: () => {
						logDebug('[GraphQL] WebSocket closed after token update');
					},
					error: (error) => {
						logDebugError(
							`[GraphQL] WebSocket error after token update connecting to EXPLICIT URL ${baseWsEndpoint}`,
							error
						);
					},
				},
			});

			wsClient = newWsClient;

			// Update the wsLink with new client
			const newWsLink = new GraphQLWsLink(wsClient);
			logDebug(`[GraphQL] Recreated wsLink with EXPLICIT URL: ${wsUrl}`);

			// Recreate split link with new WebSocket link
			const newSplitLink = split(
				({ query }) => {
					const definition = getMainDefinition(query);
					return (
						definition.kind === 'OperationDefinition' && definition.operation === 'subscription'
					);
				},
				newWsLink,
				newHttpLink
			);

			// Recreate combined link
			const newLink = from([errorLink, retryLink, newSplitLink]);

			// Update Apollo Client link
			client.setLink(newLink);

			logDebug('[GraphQL] Token updated, cache cleared, WebSocket reconnected');
		} else {
			// No WebSocket - just update HTTP link
			const newLink = from([errorLink, retryLink, newHttpLink]);
			client.setLink(newLink);
			logDebug('[GraphQL] Token updated, cache cleared (no WebSocket to reconnect)');
		}

		// Clear cache on token change
		void client.clearStore();
	};

	// Cleanup function
	const close = () => {
		if (wsClient) {
			wsClient.dispose();
		}
		client.stop();
		logDebug('[GraphQL] Client closed');
	};

	return {
		client,
		wsClient, // null if wsEndpoint not provided
		updateToken,
		close,
	};
}

/**
 * Singleton client instance (optional)
 */
let clientInstance: GraphQLClientInstance | null = null;

/**
 * Get or create singleton GraphQL client
 */
export function getGraphQLClient(config?: GraphQLClientConfig): GraphQLClientInstance {
	if (!clientInstance && !config) {
		throw new Error('GraphQL client not initialized. Call getGraphQLClient with config first.');
	}

	if (config && !clientInstance) {
		clientInstance = createGraphQLClient(config);
	}

	if (!clientInstance) {
		throw new Error('GraphQL client not initialized.');
	}

	return clientInstance;
}

/**
 * Close and reset singleton client
 */
export function closeGraphQLClient(): void {
	if (clientInstance) {
		clientInstance.close();
		clientInstance = null;
	}
}
