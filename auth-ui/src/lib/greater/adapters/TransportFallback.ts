import { SseClient } from './SseClient';
import { HttpPollingClient } from './HttpPollingClient';
import { resolveLogger } from './logger.js';
import type {
	TransportFallbackConfig,
	TransportAdapter,
	WebSocketEventHandler,
	TransportFallbackState,
	TransportLogger,
	SseClientState,
	HttpPollingClientState,
	WebSocketEvent,
} from './types';

/**
 * Transport client with automatic fallback from SSE to HTTP Polling
 */
export class TransportFallback implements TransportAdapter<TransportFallbackState> {
	private readonly config: TransportFallbackConfig & {
		autoFallback: boolean;
		forceTransport: 'sse' | 'polling' | 'auto';
		logger: TransportLogger;
	};
	private currentTransport:
		| TransportAdapter<SseClientState>
		| TransportAdapter<HttpPollingClientState>
		| null = null;
	private transportType: 'sse' | 'polling' | null = null;
	private eventHandlers: Map<string, Set<WebSocketEventHandler>> = new Map();
	private unsubscribers: Map<string, (() => void)[]> = new Map();
	private isDestroyed = false;
	private fallbackAttempted = false;
	private readonly logger: Required<TransportLogger>;

	constructor(config: TransportFallbackConfig) {
		this.logger = resolveLogger(config.logger);
		this.config = {
			...config,
			primary: { ...config.primary },
			fallback: { ...config.fallback },
			autoFallback: config.autoFallback !== false,
			forceTransport: config.forceTransport ?? 'auto',
			logger: config.logger ?? this.logger,
		};

		this.config.primary.logger = this.config.primary.logger ?? this.logger;
		this.config.fallback.logger = this.config.fallback.logger ?? this.logger;
	}

	/**
	 * Connect using the appropriate transport
	 */
	connect(): void {
		if (this.isDestroyed) {
			throw new Error('TransportFallback has been destroyed');
		}

		if (this.currentTransport) {
			return; // Already connected
		}

		// Determine which transport to use
		const transport = this.selectTransport();

		if (transport === 'sse') {
			this.connectSse();
		} else {
			this.connectPolling();
		}
	}

	/**
	 * Disconnect the current transport
	 */
	disconnect(): void {
		if (this.currentTransport) {
			this.currentTransport.disconnect();
			this.cleanupTransport();
		}
	}

	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void {
		this.isDestroyed = true;
		if (this.currentTransport) {
			this.currentTransport.destroy();
			this.cleanupTransport();
		}
		this.eventHandlers.clear();
	}

	/**
	 * Send a message through the current transport
	 */
	send(message: unknown): void {
		if (!this.currentTransport) {
			throw new Error('No transport connected');
		}

		if (this.currentTransport.send) {
			this.currentTransport.send(message);
		} else {
			throw new Error('Current transport does not support sending messages');
		}
	}

	/**
	 * Subscribe to events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void {
		// Store handler in our map
		let handlers = this.eventHandlers.get(event);
		if (!handlers) {
			handlers = new Set<WebSocketEventHandler>();
			this.eventHandlers.set(event, handlers);
		}
		handlers.add(handler);

		// Subscribe to current transport if connected
		if (this.currentTransport) {
			const unsubscribe = this.currentTransport.on(event, handler);
			this.addUnsubscribers(event, unsubscribe);
		}

		// Return unsubscribe function
		return () => {
			const storedHandlers = this.eventHandlers.get(event);
			if (storedHandlers) {
				storedHandlers.delete(handler);
				if (storedHandlers.size === 0) {
					this.eventHandlers.delete(event);
				}
			}

			// Unsubscribe from transport
			const unsubs = this.unsubscribers.get(event);
			if (unsubs) {
				unsubs.forEach((unsub) => unsub());
				this.unsubscribers.delete(event);
			}
		};
	}

	/**
	 * Get the current state
	 */
	getState(): TransportFallbackState {
		if (!this.currentTransport || !this.transportType) {
			return {
				status: 'disconnected',
				transport: null,
				error: null,
			};
		}

		if (this.transportType === 'sse') {
			const state = this.currentTransport.getState() as Readonly<SseClientState>;
			return {
				...state,
				transport: 'sse',
			};
		}

		const state = this.currentTransport.getState() as Readonly<HttpPollingClientState>;
		return {
			...state,
			transport: 'polling',
		};
	}

	/**
	 * Get the current transport type
	 */
	getTransportType(): 'sse' | 'polling' | null {
		return this.transportType;
	}

	/**
	 * Check if SSE is supported
	 */
	static isSseSupported(): boolean {
		return SseClient.isSupported();
	}

	private selectTransport(): 'sse' | 'polling' {
		// Check if transport is forced
		if (this.config.forceTransport && this.config.forceTransport !== 'auto') {
			return this.config.forceTransport;
		}

		// Auto-detect: Use SSE if supported, otherwise polling
		if (SseClient.isSupported() && !this.fallbackAttempted) {
			return 'sse';
		}

		return 'polling';
	}

	private connectSse(): void {
		const instantiateClient = (): TransportAdapter<SseClientState> =>
			new SseClient(this.config.primary);
		let sseClient: TransportAdapter<SseClientState> | null = null;

		try {
			sseClient = instantiateClient();
		} catch (error) {
			this.logger.warn('Failed to instantiate SSE client', {
				error,
			});
			if (this.config.autoFallback) {
				this.fallbackToPolling();
				return;
			}

			// Retry once for manual mode to recover from transient constructor errors
			try {
				sseClient = instantiateClient();
			} catch (retryError) {
				this.cleanupTransport();
				throw retryError;
			}
		}

		this.transportType = 'sse';
		this.currentTransport = sseClient;

		// Subscribe existing handlers
		this.subscribeHandlers();

		if (this.config.autoFallback && this.currentTransport) {
			const errorUnsubscribe = this.currentTransport.on('error', (event) => {
				if (event.error && this.shouldFallback(event.error)) {
					errorUnsubscribe();
					this.fallbackToPolling();
				}
			});

			this.addUnsubscribers('_fallback_error', errorUnsubscribe);
		}

		if (!this.currentTransport) {
			return;
		}

		this.currentTransport.connect();
	}

	private connectPolling(): void {
		this.transportType = 'polling';
		this.currentTransport = new HttpPollingClient(this.config.fallback);

		// Subscribe existing handlers
		this.subscribeHandlers();

		if (!this.currentTransport) {
			return;
		}

		this.currentTransport.connect();
	}

	private addUnsubscribers(event: string, ...entries: Array<() => void>): void {
		const existing = this.unsubscribers.get(event);
		if (existing) {
			existing.push(...entries);
			return;
		}
		this.unsubscribers.set(event, [...entries]);
	}

	private fallbackToPolling(): void {
		if (this.fallbackAttempted || this.transportType === 'polling') {
			return; // Already using polling or fallback already attempted
		}

		this.fallbackAttempted = true;
		this.logger.warn('SSE connection failed, switching to HTTP polling');

		// Disconnect current transport
		if (this.currentTransport) {
			this.currentTransport.disconnect();
			this.cleanupTransport();
		}

		// Connect with polling
		this.connectPolling();

		// Emit fallback event
		this.emitToHandlers('fallback', { from: 'sse', to: 'polling' });
	}

	private shouldFallback(error?: Error): boolean {
		if (!error) return false;

		// Fallback on specific error conditions
		const errorMessage = error.message.toLowerCase();

		// Network errors
		if (
			errorMessage.includes('network') ||
			errorMessage.includes('failed to fetch') ||
			errorMessage.includes('connection')
		) {
			return true;
		}

		// SSE not supported errors
		if (errorMessage.includes('eventsource') || errorMessage.includes('not supported')) {
			return true;
		}

		// Server errors that indicate SSE endpoint issues
		if (
			errorMessage.includes('404') ||
			errorMessage.includes('405') ||
			errorMessage.includes('501')
		) {
			return true;
		}

		return false;
	}

	private subscribeHandlers(): void {
		const adapter = this.currentTransport;
		if (!adapter) return;

		// Subscribe all existing handlers to the new transport
		this.eventHandlers.forEach((handlers, event) => {
			handlers.forEach((handler) => {
				const unsubscribe = adapter.on(event, handler);
				this.addUnsubscribers(event, unsubscribe);
			});
		});
	}

	private cleanupTransport(): void {
		// Unsubscribe all handlers
		this.unsubscribers.forEach((unsubs) => {
			unsubs.forEach((unsub) => unsub());
		});
		this.unsubscribers.clear();

		this.currentTransport = null;
		this.transportType = null;
	}

	private emitToHandlers(event: string, data?: unknown): void {
		const handlers = this.eventHandlers.get(event);
		if (!handlers) {
			return;
		}

		const wsEvent: WebSocketEvent = {
			type: event,
			data,
		};

		handlers.forEach((handler) => {
			try {
				handler(wsEvent);
			} catch (error) {
				this.logger.error(`Error in fallback event handler for ${event}`, { error });
			}
		});
	}
}
