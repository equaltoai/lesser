import { resolveLogger } from './logger.js';
import type {
	HttpPollingClientConfig,
	HttpPollingClientState,
	HttpPollingMessage,
	WebSocketEvent,
	WebSocketEventHandler,
	LatencySample,
	TransportAdapter,
	TransportLogger,
} from './types';

/**
 * HTTP Polling client with automatic reconnection and latency tracking
 */
export class HttpPollingClient implements TransportAdapter<HttpPollingClientState> {
	private config: Required<HttpPollingClientConfig>;
	private state: HttpPollingClientState;
	private eventHandlers: Map<string, Set<WebSocketEventHandler>> = new Map();
	private pollingTimer: ReturnType<typeof setTimeout> | null = null;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private latencySamples: LatencySample[] = [];
	private isDestroyed = false;
	private isExplicitDisconnect = false;
	private abortController: AbortController | null = null;
	private consecutiveErrors = 0;
	private maxConsecutiveErrors = 3;
	private readonly logger: Required<TransportLogger>;

	constructor(config: HttpPollingClientConfig) {
		this.logger = resolveLogger(config.logger);
		this.config = {
			url: config.url,
			authToken: config.authToken || '',
			pollingInterval: config.pollingInterval || 5000,
			initialReconnectDelay: config.initialReconnectDelay || 500,
			maxReconnectDelay: config.maxReconnectDelay || 30000,
			jitterFactor: config.jitterFactor || 0.3,
			maxReconnectAttempts: config.maxReconnectAttempts || Infinity,
			lastEventIdStorageKey: config.lastEventIdStorageKey || 'polling_last_event_id',
			storage:
				config.storage ||
				((typeof window !== 'undefined' ? window.localStorage : undefined) as Storage),
			withCredentials: config.withCredentials || false,
			headers: config.headers || {},
			requestTimeout: config.requestTimeout || 30000,
			enableLatencySampling: config.enableLatencySampling !== false,
			logger: config.logger ?? this.logger,
		};

		this.state = {
			status: 'disconnected',
			reconnectAttempts: 0,
			latency: null,
			error: null,
			lastEventId: this.loadLastEventId(),
		};
	}

	/**
	 * Start polling
	 */
	connect(): void {
		if (this.isDestroyed) {
			throw new Error('HttpPollingClient has been destroyed');
		}

		if (this.state.status === 'polling' || this.state.status === 'waiting') {
			return;
		}

		this.isExplicitDisconnect = false;
		this.consecutiveErrors = 0;
		this.cleanup();
		this.setState({ status: 'polling', error: null, reconnectAttempts: 0 });

		this.emit('open', {});
		this.startPolling();
	}

	/**
	 * Stop polling
	 */
	disconnect(): void {
		this.isExplicitDisconnect = true;
		this.setState({ status: 'disconnected' });
		this.cleanup();
		this.emit('close', {});
	}

	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void {
		this.isDestroyed = true;
		this.cleanup();
		this.eventHandlers.clear();
	}

	/**
	 * Send a message (HTTP POST)
	 */
	async send(message: HttpPollingMessage): Promise<void> {
		if (this.state.status !== 'polling' && this.state.status !== 'waiting') {
			throw new Error('HttpPollingClient is not connected');
		}

		const messageWithTimestamp = {
			...message,
			timestamp: message.timestamp || Date.now(),
		};

		const url = new URL(this.config.url);
		url.pathname = url.pathname.replace(/\/?$/, '/send');

		const headers: HeadersInit = {
			'Content-Type': 'application/json',
			...this.config.headers,
		};

		if (this.config.authToken) {
			headers['Authorization'] = `Bearer ${this.config.authToken}`;
		}

		try {
			const response = await fetch(url.toString(), {
				method: 'POST',
				headers,
				body: JSON.stringify(messageWithTimestamp),
				credentials: this.config.withCredentials ? 'include' : 'same-origin',
				signal: AbortSignal.timeout(this.config.requestTimeout),
			});

			if (!response.ok) {
				throw new Error(`Failed to send message: ${response.status} ${response.statusText}`);
			}
		} catch (error) {
			const resolvedError = this.resolveError(error);
			this.handleError(resolvedError);
			throw resolvedError;
		}
	}

	/**
	 * Subscribe to events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void {
		let handlers = this.eventHandlers.get(event);
		if (!handlers) {
			handlers = new Set<WebSocketEventHandler>();
			this.eventHandlers.set(event, handlers);
		}
		handlers.add(handler);

		// Return unsubscribe function
		return () => {
			const storedHandlers = this.eventHandlers.get(event);
			if (storedHandlers) {
				storedHandlers.delete(handler);
				if (storedHandlers.size === 0) {
					this.eventHandlers.delete(event);
				}
			}
		};
	}

	/**
	 * Get the current state
	 */
	getState(): Readonly<HttpPollingClientState> {
		return { ...this.state };
	}

	/**
	 * Get average latency from recent samples
	 */
	getAverageLatency(): number | null {
		if (this.latencySamples.length === 0) {
			return null;
		}

		const sum = this.latencySamples.reduce((acc, sample) => acc + sample.latency, 0);
		return Math.round(sum / this.latencySamples.length);
	}

	private startPolling(): void {
		if (this.isDestroyed || this.isExplicitDisconnect) {
			return;
		}

		this.poll();
	}

	private async poll(): Promise<void> {
		if (this.isDestroyed || this.isExplicitDisconnect) {
			return;
		}

		this.abortController = new AbortController();
		const startTime = Date.now();

		try {
			const url = new URL(this.config.url);
			url.pathname = url.pathname.replace(/\/?$/, '/poll');

			// Add query parameters
			if (this.config.authToken) {
				url.searchParams.set('token', this.config.authToken);
			}

			if (this.state.lastEventId) {
				url.searchParams.set('lastEventId', this.state.lastEventId);
			}

			const headers: HeadersInit = {
				Accept: 'application/json',
				...this.config.headers,
			};

			if (this.config.authToken && !url.searchParams.has('token')) {
				headers['Authorization'] = `Bearer ${this.config.authToken}`;
			}

			const timeoutSignal = AbortSignal.timeout(this.config.requestTimeout);
			const combinedSignal = this.combineSignals([this.abortController.signal, timeoutSignal]);

			const response = await fetch(url.toString(), {
				method: 'GET',
				headers,
				credentials: this.config.withCredentials ? 'include' : 'same-origin',
				signal: combinedSignal,
			});

			if (!response.ok) {
				throw new Error(`Polling failed: ${response.status} ${response.statusText}`);
			}

			const data = await response.json();

			// Calculate latency
			const latency = Date.now() - startTime;
			if (this.config.enableLatencySampling) {
				this.addLatencySample(latency);
				this.setState({ latency });
				this.emit('latency', { latency });
			}

			// Reset consecutive errors on successful poll
			this.consecutiveErrors = 0;

			// Process messages
			if (Array.isArray(data)) {
				for (const message of data) {
					this.handleMessage(message);
				}
			} else if (data) {
				this.handleMessage(data);
			}

			// Continue polling
			this.setState({ status: 'waiting' });
			this.scheduleNextPoll();
		} catch (error) {
			if (this.isAbortError(error)) {
				return; // Intentional abort
			}

			this.consecutiveErrors++;
			const resolvedError = this.resolveError(error);
			this.handleError(resolvedError);

			if (this.consecutiveErrors >= this.maxConsecutiveErrors) {
				this.handleDisconnection();
			} else {
				this.scheduleNextPoll();
			}
		} finally {
			this.abortController = null;
		}
	}

	private combineSignals(signals: AbortSignal[]): AbortSignal {
		const controller = new AbortController();

		for (const signal of signals) {
			if (signal.aborted) {
				controller.abort();
				break;
			}

			signal.addEventListener('abort', () => controller.abort(), { once: true });
		}

		return controller.signal;
	}

	private scheduleNextPoll(): void {
		if (this.isDestroyed || this.isExplicitDisconnect) {
			return;
		}

		// Apply jitter to polling interval to prevent thundering herd
		const jitter = this.config.pollingInterval * 0.1 * Math.random();
		const delay = Math.round(this.config.pollingInterval + jitter);

		this.pollingTimer = setTimeout(() => {
			this.pollingTimer = null;
			this.setState({ status: 'polling' });
			this.poll();
		}, delay);
	}

	private handleMessage(message: HttpPollingMessage): void {
		try {
			// Update lastEventId if present
			if (message.id) {
				this.setState({ lastEventId: message.id });
				this.saveLastEventId(message.id);
			}

			// Emit the message
			this.emit('message', message);

			// Also emit type-specific event if type is present
			if (message.type && message.data !== undefined) {
				this.emit(message.type, message.data);
			}
		} catch (error) {
			this.handleError(new Error(`Failed to process message: ${error}`));
		}
	}

	private handleError(error: Error): void {
		this.setState({ error });
		this.logger.error('HTTP polling transport error', { error });
		this.emit('error', { error }, error);
	}

	private resolveError(error: unknown): Error {
		return error instanceof Error ? error : new Error(String(error));
	}

	private isAbortError(error: unknown): error is { name: string } {
		return (
			typeof error === 'object' &&
			error !== null &&
			(error as { name?: unknown }).name === 'AbortError'
		);
	}

	private handleDisconnection(): void {
		this.cleanup();

		if (!this.isDestroyed && !this.isExplicitDisconnect) {
			if (this.state.reconnectAttempts < this.config.maxReconnectAttempts) {
				this.setState({ status: 'reconnecting' });
				this.scheduleReconnect();
			} else {
				this.setState({ status: 'disconnected' });
				this.emit('close', {});
			}
		}
	}

	private scheduleReconnect(): void {
		if (this.reconnectTimer || this.isDestroyed) {
			return;
		}

		const attempts = this.state.reconnectAttempts;

		if (attempts >= this.config.maxReconnectAttempts) {
			this.setState({ status: 'disconnected' });
			this.emit('close', {});
			return;
		}

		// Calculate delay with exponential backoff
		const baseDelay = Math.min(
			this.config.initialReconnectDelay * Math.pow(2, attempts),
			this.config.maxReconnectDelay
		);

		// Add jitter to prevent thundering herd
		const jitter = baseDelay * this.config.jitterFactor * Math.random();
		const delay = Math.round(baseDelay + jitter);

		this.setState({ reconnectAttempts: attempts + 1 });
		this.emit('reconnecting', { attempt: attempts + 1, delay });

		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			this.consecutiveErrors = 0;
			this.connect();

			// Emit reconnected event if successful
			if (this.state.status === 'polling' || this.state.status === 'waiting') {
				this.emit('reconnected', {});
			}
		}, delay);
	}

	private addLatencySample(latency: number): void {
		const sample: LatencySample = {
			timestamp: Date.now(),
			latency,
		};

		this.latencySamples.push(sample);

		// Keep only last 10 samples
		if (this.latencySamples.length > 10) {
			this.latencySamples.shift();
		}
	}

	private cleanup(): void {
		if (this.abortController) {
			this.abortController.abort();
			this.abortController = null;
		}

		if (this.pollingTimer) {
			clearTimeout(this.pollingTimer);
			this.pollingTimer = null;
		}

		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
	}

	private setState(updates: Partial<HttpPollingClientState>): void {
		this.state = { ...this.state, ...updates };
	}

	private emit(event: string, data?: unknown, error?: Error): void {
		const handlers = this.eventHandlers.get(event);
		if (handlers) {
			const wsEvent: WebSocketEvent = {
				type: event,
				data,
				error,
			};

			handlers.forEach((handler) => {
				try {
					handler(wsEvent);
				} catch (err) {
					this.logger.error(`Error in HTTP polling event handler for ${event}`, { error: err });
				}
			});
		}
	}

	private loadLastEventId(): string | null {
		if (!this.config.storage) {
			return null;
		}

		try {
			return this.config.storage.getItem(this.config.lastEventIdStorageKey);
		} catch {
			return null;
		}
	}

	private saveLastEventId(eventId: string): void {
		if (!this.config.storage) {
			return;
		}

		try {
			this.config.storage.setItem(this.config.lastEventIdStorageKey, eventId);
		} catch {
			// Ignore storage errors
		}
	}
}
