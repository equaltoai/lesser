import { resolveLogger } from './logger.js';
import type {
	SseClientConfig,
	SseClientState,
	WebSocketEvent,
	WebSocketEventHandler,
	HeartbeatMessage,
	LatencySample,
	TransportAdapter,
	TransportLogger,
} from './types';

type TransportMessage = {
	type?: string;
	data?: unknown;
	timestamp?: number;
	[key: string]: unknown;
};

/**
 * Server-Sent Events client with automatic reconnection, heartbeat, and latency sampling
 */
export class SseClient implements TransportAdapter<SseClientState> {
	private config: Required<SseClientConfig>;
	private eventSource: EventSource | null = null;
	private state: SseClientState;
	private eventHandlers: Map<string, Set<WebSocketEventHandler>> = new Map();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private heartbeatTimer: ReturnType<typeof setTimeout> | null = null;
	private heartbeatTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
	private latencySamplingTimer: ReturnType<typeof setTimeout> | null = null;
	private latencySamples: LatencySample[] = [];
	private pendingPings: Map<number, number> = new Map();
	private isDestroyed = false;
	private isExplicitDisconnect = false;
	private abortController: AbortController | null = null;
	private readonly logger: Required<TransportLogger>;

	constructor(config: SseClientConfig) {
		this.logger = resolveLogger(config.logger);
		this.config = {
			url: config.url,
			authToken: config.authToken || '',
			heartbeatInterval: config.heartbeatInterval || 30000,
			heartbeatTimeout: config.heartbeatTimeout || 60000,
			initialReconnectDelay: config.initialReconnectDelay || 500,
			maxReconnectDelay: config.maxReconnectDelay || 30000,
			jitterFactor: config.jitterFactor ?? 0.3,
			maxReconnectAttempts: config.maxReconnectAttempts || Infinity,
			enableLatencySampling: config.enableLatencySampling !== false,
			latencySamplingInterval: config.latencySamplingInterval || 10000,
			lastEventIdStorageKey: config.lastEventIdStorageKey || 'sse_last_event_id',
			storage:
				config.storage ||
				((typeof window !== 'undefined' ? window.localStorage : undefined) as Storage),
			withCredentials: config.withCredentials || false,
			headers: config.headers || {},
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
	 * Check if EventSource is supported
	 */
	static isSupported(): boolean {
		return typeof EventSource !== 'undefined';
	}

	/**
	 * Connect to the SSE endpoint
	 */
	connect(): void {
		if (this.isDestroyed) {
			throw new Error('SseClient has been destroyed');
		}

		if (this.eventSource?.readyState === EventSource.OPEN) {
			return;
		}

		this.isExplicitDisconnect = false;
		this.cleanup();
		this.setState({ status: 'connecting', error: null });

		try {
			const url = new URL(this.config.url);

			// Add auth token as query parameter if provided
			if (this.config.authToken) {
				url.searchParams.set('token', this.config.authToken);
			}

			// Add last event ID for resumption if available
			if (this.state.lastEventId) {
				url.searchParams.set('lastEventId', this.state.lastEventId);
			}

			// Create EventSource with configuration
			const eventSourceInit: EventSourceInit = {
				withCredentials: this.config.withCredentials,
			};

			// For custom headers, we need to use a polyfill or fetch-based approach
			// Standard EventSource doesn't support custom headers
			if (Object.keys(this.config.headers).length > 0) {
				this.connectWithFetch(url.toString());
			} else {
				this.eventSource = new EventSource(url.toString(), eventSourceInit);
				this.setupEventListeners();
			}
		} catch (error) {
			this.handleError(error);
			this.scheduleReconnect();
		}
	}

	/**
	 * Connect using fetch for custom headers support
	 */
	private async connectWithFetch(url: string): Promise<void> {
		this.abortController = new AbortController();

		try {
			const headers: HeadersInit = {
				Accept: 'text/event-stream',
				'Cache-Control': 'no-cache',
				...this.config.headers,
			};

			if (this.config.authToken) {
				headers['Authorization'] = `Bearer ${this.config.authToken}`;
			}

			if (this.state.lastEventId) {
				headers['Last-Event-ID'] = this.state.lastEventId;
			}

			const response = await fetch(url, {
				method: 'GET',
				headers,
				credentials: this.config.withCredentials ? 'include' : 'same-origin',
				signal: this.abortController.signal,
			});

			if (!response.ok) {
				throw new Error(`SSE connection failed: ${response.status} ${response.statusText}`);
			}

			if (!response.body) {
				throw new Error('Response body is empty');
			}

			const reader = response.body.getReader();
			const decoder = new TextDecoder();
			let buffer = '';

			this.handleOpen();

			while (true) {
				const { done, value } = await reader.read();

				if (done) {
					break;
				}

				buffer += decoder.decode(value, { stream: true });
				const lines = buffer.split('\n');

				// Keep the last incomplete line in the buffer
				buffer = lines.pop() || '';

				for (const line of lines) {
					this.processLine(line);
				}
			}

			// Connection closed
			this.handleClose();
		} catch (error) {
			if (this.isAbortError(error)) {
				return; // Intentional disconnect
			}
			const resolvedError = error instanceof Error ? error : new Error(String(error));
			this.handleError(resolvedError);
			this.scheduleReconnect();
		}
	}

	/**
	 * Process a single SSE line
	 */
	private processLine(line: string): void {
		if (line.startsWith('data: ')) {
			const data = line.slice(6);
			this.handleMessage(data);
		} else if (line.startsWith('event: ')) {
			// Handle named events if needed
			const eventType = line.slice(7);
			// Store for next data line
			this.pendingEventType = eventType;
		} else if (line.startsWith('id: ')) {
			const id = line.slice(4);
			this.setState({ lastEventId: id });
			this.saveLastEventId(id);
		} else if (line.startsWith('retry: ')) {
			const retry = parseInt(line.slice(7), 10);
			if (!isNaN(retry)) {
				// Update reconnect delay based on server suggestion
				this.serverSuggestedRetry = retry;
			}
		}
	}

	private pendingEventType?: string;
	private serverSuggestedRetry?: number;

	/**
	 * Disconnect from the SSE endpoint
	 */
	disconnect(): void {
		this.isExplicitDisconnect = true;
		this.setState({ status: 'disconnected' });
		this.cleanup();
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
	 * Subscribe to SSE events
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
			const handlers = this.eventHandlers.get(event);
			if (handlers) {
				handlers.delete(handler);
				if (handlers.size === 0) {
					this.eventHandlers.delete(event);
				}
			}
		};
	}

	/**
	 * Get the current state
	 */
	getState(): Readonly<SseClientState> {
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

	private setupEventListeners(): void {
		if (!this.eventSource) return;

		this.eventSource.addEventListener('open', this.handleOpen.bind(this));
		this.eventSource.addEventListener('error', this.handleError.bind(this));
		this.eventSource.addEventListener('message', (event) => {
			this.handleEventSourceData(event);
		});

		// Listen for custom event types
		this.eventSource.addEventListener('ping', (event) => {
			this.handleEventSourceData(event, 'ping');
		});

		this.eventSource.addEventListener('pong', (event) => {
			this.handleEventSourceData(event, 'pong');
		});
	}

	private handleEventSourceData(event: Event, overrideType?: string): void {
		const messageEvent = event as MessageEvent<string>;
		this.handleMessage(messageEvent.data, messageEvent.lastEventId, overrideType);
	}

	private handleOpen(): void {
		const wasReconnecting = this.state.reconnectAttempts > 0;

		this.setState({
			status: 'connected',
			reconnectAttempts: 0,
			error: null,
		});

		this.startHeartbeat();

		if (this.config.enableLatencySampling) {
			this.startLatencySampling();
		}

		this.emit('open', {});

		if (wasReconnecting) {
			this.emit('reconnected', {});
		}
	}

	private handleClose(): void {
		const wasConnected = this.state.status === 'connected';

		this.cleanup();

		if (!this.isDestroyed && !this.isExplicitDisconnect) {
			if (wasConnected && this.state.reconnectAttempts < this.config.maxReconnectAttempts) {
				this.setState({ status: 'reconnecting' });
				this.scheduleReconnect();
			} else if (!wasConnected) {
				// Connection failed during initial connect
				this.setState({ status: 'disconnected' });
				if (this.state.reconnectAttempts < this.config.maxReconnectAttempts) {
					this.scheduleReconnect();
				}
			} else {
				this.setState({ status: 'disconnected' });
			}
		}

		this.emit('close', {});
	}

	private handleError(error: unknown): void {
		const errorObj = error instanceof Error ? error : new Error('SSE error');

		this.setState({ error: errorObj });
		this.logger.error('SSE transport error', { error: errorObj });
		this.emit('error', { error: errorObj }, errorObj);

		// EventSource will automatically reconnect on error
		// But we want to control the reconnection logic
		if (typeof EventSource !== 'undefined' && this.eventSource?.readyState === EventSource.CLOSED) {
			this.handleClose();
		}
	}

	private handleMessage(data: string, lastEventId?: string, eventType?: string): void {
		try {
			// Update lastEventId if present
			if (lastEventId) {
				this.setState({ lastEventId });
				this.saveLastEventId(lastEventId);
			}

			const fallbackType = eventType ?? this.pendingEventType ?? 'message';
			const message = this.parseMessage(data, fallbackType);

			// Clear pending event type after use
			this.pendingEventType = undefined;

			// Handle heartbeat messages
			if (message.type === 'pong') {
				const heartbeat = this.toHeartbeatMessage(message);
				if (heartbeat) {
					this.handlePong(heartbeat);
				}
				return;
			}

			// Emit the message
			this.emit('message', message);

			// Also emit type-specific event if type is present
			const messageType = typeof message.type === 'string' ? message.type : undefined;
			if (messageType && Object.prototype.hasOwnProperty.call(message, 'data')) {
				this.emit(messageType, message.data);
			}
		} catch (error) {
			const resolvedError = error instanceof Error ? error : new Error(String(error));
			this.handleError(new Error(`Failed to process message: ${resolvedError.message}`));
		}
	}

	private parseMessage(data: string, fallbackType: string): TransportMessage {
		try {
			const parsed = JSON.parse(data) as unknown;
			if (this.isTransportMessage(parsed)) {
				return {
					...parsed,
					type: typeof parsed.type === 'string' ? parsed.type : fallbackType,
				};
			}

			return {
				type: fallbackType,
				data: parsed,
			};
		} catch {
			return {
				type: fallbackType,
				data,
			};
		}
	}

	private isTransportMessage(value: unknown): value is TransportMessage {
		return typeof value === 'object' && value !== null;
	}

	private toHeartbeatMessage(message: TransportMessage): HeartbeatMessage | null {
		if (
			(message.type === 'ping' || message.type === 'pong') &&
			typeof message.timestamp === 'number'
		) {
			return {
				type: message.type,
				timestamp: message.timestamp,
			};
		}

		return null;
	}

	private isAbortError(error: unknown): error is { name: string } {
		return (
			typeof error === 'object' &&
			error !== null &&
			(error as { name?: unknown }).name === 'AbortError'
		);
	}

	private startHeartbeat(): void {
		this.stopHeartbeat();

		this.heartbeatTimer = setInterval(() => {
			if (this.eventSource?.readyState === EventSource.OPEN || this.abortController) {
				const timestamp = Date.now();
				const ping: HeartbeatMessage = { type: 'ping', timestamp };

				// Track pending ping for latency measurement
				this.pendingPings.set(timestamp, Date.now());

				// Send ping through a separate channel (e.g., POST request)
				this.sendPing(ping);

				// Set timeout for heartbeat response
				this.heartbeatTimeoutTimer = setTimeout(() => {
					this.handleError(new Error('Heartbeat timeout'));
					this.eventSource?.close();
					this.abortController?.abort();
				}, this.config.heartbeatTimeout);
			}
		}, this.config.heartbeatInterval);
	}

	private async sendPing(ping: HeartbeatMessage): Promise<void> {
		try {
			const url = new URL(this.config.url);
			url.pathname = url.pathname.replace(/\/?$/, '/ping');

			const headers: HeadersInit = {
				'Content-Type': 'application/json',
				...this.config.headers,
			};

			if (this.config.authToken) {
				headers['Authorization'] = `Bearer ${this.config.authToken}`;
			}

			await fetch(url.toString(), {
				method: 'POST',
				headers,
				body: JSON.stringify(ping),
				credentials: this.config.withCredentials ? 'include' : 'same-origin',
			});
		} catch (error) {
			this.logger.debug('SSE latency ping request failed', { error });
		}
	}

	private stopHeartbeat(): void {
		if (this.heartbeatTimer) {
			clearInterval(this.heartbeatTimer);
			this.heartbeatTimer = null;
		}

		if (this.heartbeatTimeoutTimer) {
			clearTimeout(this.heartbeatTimeoutTimer);
			this.heartbeatTimeoutTimer = null;
		}

		this.pendingPings.clear();
	}

	private handlePong(message: HeartbeatMessage): void {
		// Clear heartbeat timeout
		if (this.heartbeatTimeoutTimer) {
			clearTimeout(this.heartbeatTimeoutTimer);
			this.heartbeatTimeoutTimer = null;
		}

		// Calculate latency
		const sentTime = this.pendingPings.get(message.timestamp);
		if (sentTime) {
			const latency = Date.now() - sentTime;
			this.pendingPings.delete(message.timestamp);

			// Update state with current latency
			this.setState({ latency });

			// Add to samples for averaging
			this.addLatencySample(latency);

			// Emit latency event
			this.emit('latency', { latency });
		}
	}

	private startLatencySampling(): void {
		this.stopLatencySampling();

		this.latencySamplingTimer = setInterval(() => {
			if (this.eventSource?.readyState === EventSource.OPEN || this.abortController) {
				// Send a ping for latency measurement
				const timestamp = Date.now();
				const ping: HeartbeatMessage = { type: 'ping', timestamp };

				this.pendingPings.set(timestamp, Date.now());
				this.sendPing(ping);
			}
		}, this.config.latencySamplingInterval);
	}

	private stopLatencySampling(): void {
		if (this.latencySamplingTimer) {
			clearInterval(this.latencySamplingTimer);
			this.latencySamplingTimer = null;
		}
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

	private scheduleReconnect(): void {
		if (this.reconnectTimer || this.isDestroyed) {
			return;
		}

		const attempts = this.state.reconnectAttempts;

		if (attempts >= this.config.maxReconnectAttempts) {
			this.setState({ status: 'disconnected' });
			return;
		}

		// Use server-suggested retry if available, otherwise exponential backoff
		let baseDelay: number;
		if (this.serverSuggestedRetry !== undefined) {
			baseDelay = this.serverSuggestedRetry;
			this.serverSuggestedRetry = undefined; // Use it once
		} else {
			baseDelay = Math.min(
				this.config.initialReconnectDelay * Math.pow(2, attempts),
				this.config.maxReconnectDelay
			);
		}

		// Add jitter to prevent thundering herd
		const jitter = baseDelay * this.config.jitterFactor * Math.random();
		const delay = Math.round(baseDelay + jitter);

		this.setState({ reconnectAttempts: attempts + 1 });
		this.emit('reconnecting', { attempt: attempts + 1, delay });

		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			this.connect();
		}, delay);
	}

	private cleanup(): void {
		if (this.eventSource) {
			this.eventSource.close();
			this.eventSource = null;
		}

		if (this.abortController) {
			this.abortController.abort();
			this.abortController = null;
		}

		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}

		this.stopHeartbeat();
		this.stopLatencySampling();
	}

	private setState(updates: Partial<SseClientState>): void {
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
					this.logger.error(`Error in SSE event handler for ${event}`, { error: err });
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
