import { resolveLogger } from './logger.js';
import type {
	WebSocketClientConfig,
	WebSocketClientState,
	WebSocketMessage,
	WebSocketEvent,
	WebSocketEventHandler,
	HeartbeatMessage,
	LatencySample,
	TransportAdapter,
	TransportLogger,
} from './types';

/**
 * WebSocketClient with automatic reconnection, heartbeat, and latency sampling
 */
export class WebSocketClient implements TransportAdapter<WebSocketClientState> {
	private config: Required<WebSocketClientConfig>;
	private socket: WebSocket | null = null;
	private state: WebSocketClientState;
	private eventHandlers: Map<string, Set<WebSocketEventHandler>> = new Map();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private heartbeatTimer: ReturnType<typeof setTimeout> | null = null;
	private heartbeatTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
	private latencySamplingTimer: ReturnType<typeof setTimeout> | null = null;
	private latencySamples: LatencySample[] = [];
	private pendingPings: Map<number, number> = new Map();
	private isDestroyed = false;
	private isExplicitDisconnect = false;
	private readonly logger: Required<TransportLogger>;

	constructor(config: WebSocketClientConfig) {
		this.logger = resolveLogger(config.logger);
		this.config = {
			url: config.url,
			authToken: config.authToken || '',
			heartbeatInterval: config.heartbeatInterval || 30000,
			heartbeatTimeout: config.heartbeatTimeout || 60000,
			initialReconnectDelay: config.initialReconnectDelay || 500,
			maxReconnectDelay: config.maxReconnectDelay || 30000,
			jitterFactor: config.jitterFactor || 0.3,
			maxReconnectAttempts: config.maxReconnectAttempts || Infinity,
			enableLatencySampling: config.enableLatencySampling !== false,
			latencySamplingInterval: config.latencySamplingInterval || 10000,
			lastEventIdStorageKey: config.lastEventIdStorageKey || 'ws_last_event_id',
			storage:
				config.storage ||
				((typeof window !== 'undefined' ? window.localStorage : undefined) as Storage),
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
	 * Connect to the WebSocket server
	 */
	connect(): void {
		if (this.isDestroyed) {
			throw new Error('WebSocketClient has been destroyed');
		}

		if (this.socket?.readyState === WebSocket.OPEN) {
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

			this.socket = new WebSocket(url.toString());
			this.setupEventListeners();
		} catch (error) {
			const resolvedError = this.resolveError(error);
			this.handleError(resolvedError);
			this.scheduleReconnect();
		}
	}

	/**
	 * Disconnect from the WebSocket server
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
		this.setState({ status: 'disconnected' });
		this.cleanup();
		this.eventHandlers.clear();
	}

	/**
	 * Send a message through the WebSocket
	 */
	send(message: WebSocketMessage): void {
		if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
			throw new Error('WebSocket is not connected');
		}

		const messageWithTimestamp = {
			...message,
			timestamp: message.timestamp || Date.now(),
		};

		this.socket.send(JSON.stringify(messageWithTimestamp));
	}

	/**
	 * Subscribe to WebSocket events
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
	getState(): Readonly<WebSocketClientState> {
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
		if (!this.socket) return;

		this.socket.addEventListener('open', this.handleOpen.bind(this));
		this.socket.addEventListener('close', this.handleClose.bind(this));
		this.socket.addEventListener('error', this.handleError.bind(this));
		this.socket.addEventListener('message', this.handleMessage.bind(this));
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

	private handleClose(event: CloseEvent): void {
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

		this.emit('close', { code: event.code, reason: event.reason });
	}

	private handleError(error: unknown): void {
		const errorObj = this.resolveError(error);

		this.setState({ error: errorObj });
		this.logger.error('WebSocket transport error', { error: errorObj });
		this.emit('error', { error: errorObj }, errorObj);
	}

	private handleMessage(event: MessageEvent): void {
		try {
			const message = JSON.parse(event.data) as WebSocketMessage | HeartbeatMessage;

			// Handle heartbeat messages
			if ('type' in message && message.type === 'pong') {
				this.handlePong(message as HeartbeatMessage);
				return;
			}

			// Update lastEventId if present
			if ('id' in message && message.id) {
				this.setState({ lastEventId: message.id });
				this.saveLastEventId(message.id);
			}

			// Emit the message
			this.emit('message', message);

			// Also emit type-specific event if type is present
			if ('type' in message && message.type && 'data' in message) {
				this.emit(message.type, (message as WebSocketMessage).data);
			}
		} catch (error) {
			const resolvedError = this.resolveError(error);
			this.handleError(new Error(`Failed to parse message: ${resolvedError.message}`));
		}
	}

	private startHeartbeat(): void {
		this.stopHeartbeat();

		this.heartbeatTimer = setInterval(() => {
			if (this.socket?.readyState === WebSocket.OPEN) {
				const timestamp = Date.now();
				const ping: HeartbeatMessage = { type: 'ping', timestamp };

				// Track pending ping for latency measurement
				this.pendingPings.set(timestamp, Date.now());

				try {
					this.socket.send(JSON.stringify(ping));

					// Set timeout for heartbeat response
					this.heartbeatTimeoutTimer = setTimeout(() => {
						this.handleError(new Error('Heartbeat timeout'));
						this.socket?.close();
					}, this.config.heartbeatTimeout);
				} catch (error) {
					this.handleError(error);
				}
			}
		}, this.config.heartbeatInterval);
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
			if (this.socket?.readyState === WebSocket.OPEN) {
				// Send a ping for latency measurement
				const timestamp = Date.now();
				const ping: HeartbeatMessage = { type: 'ping', timestamp };

				this.pendingPings.set(timestamp, Date.now());

				try {
					this.socket.send(JSON.stringify(ping));
				} catch (error) {
					this.logger.debug('WebSocket latency sampling ping failed', { error });
				}
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
			this.connect();
		}, delay);
	}

	private cleanup(): void {
		if (this.socket) {
			this.socket.removeEventListener('open', this.handleOpen.bind(this));
			this.socket.removeEventListener('close', this.handleClose.bind(this));
			this.socket.removeEventListener('error', this.handleError.bind(this));
			this.socket.removeEventListener('message', this.handleMessage.bind(this));

			if (this.socket.readyState === WebSocket.OPEN) {
				this.socket.close();
			}

			this.socket = null;
		}

		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}

		this.stopHeartbeat();
		this.stopLatencySampling();
	}

	private setState(updates: Partial<WebSocketClientState>): void {
		this.state = { ...this.state, ...updates };
	}

	private resolveError(error: unknown): Error {
		return error instanceof Error ? error : new Error(String(error));
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
					this.logger.error(`Error in WebSocket event handler for ${event}`, { error: err });
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
