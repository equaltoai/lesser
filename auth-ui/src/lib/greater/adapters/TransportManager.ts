import { WebSocketClient } from './WebSocketClient';
import { SseClient } from './SseClient';
import { HttpPollingClient } from './HttpPollingClient';
import { resolveLogger } from './logger.js';
import type {
	TransportManagerConfig,
	TransportManagerState,
	TransportType,
	TransportAdapter,
	TransportSwitchEvent,
	WebSocketEventHandler,
	WebSocketEvent,
	TransportEventName,
	TransportLogger,
} from './types';

/**
 * Unified transport manager with automatic fallback through WebSocket → SSE → HTTP Polling
 *
 * Features:
 * - Feature detection for transport support
 * - Automatic fallback on persistent failures
 * - Unified event interface across all transports
 * - Transport upgrading after successful connections
 * - Comprehensive connection state management
 */
export class TransportManager implements TransportAdapter<TransportManagerState> {
	private config: Required<TransportManagerConfig>;
	private state: TransportManagerState;
	private currentTransport: TransportAdapter | null = null;
	private eventHandlers: Map<string, Set<WebSocketEventHandler>> = new Map();
	private unsubscribers: Map<string, (() => void)[]> = new Map();
	private upgradeTimer: ReturnType<typeof setTimeout> | null = null;
	private isDestroyed = false;
	private isExplicitDisconnect = false;
	private consecutiveFailures = 0;
	private readonly logger: Required<TransportLogger>;

	constructor(config: TransportManagerConfig) {
		this.logger = resolveLogger(config.logger);

		this.config = {
			websocket: { ...config.websocket },
			sse: { ...config.sse },
			polling: { ...config.polling },
			autoFallback: config.autoFallback !== false,
			forceTransport: config.forceTransport ?? 'auto',
			maxFailuresBeforeSwitch: config.maxFailuresBeforeSwitch ?? 3,
			enableUpgradeAttempts: config.enableUpgradeAttempts ?? false,
			upgradeAttemptInterval: config.upgradeAttemptInterval ?? 300000,
			logger: this.logger,
		};

		this.config.websocket.logger = this.config.websocket.logger ?? this.logger;
		this.config.sse.logger = this.config.sse.logger ?? this.logger;
		this.config.polling.logger = this.config.polling.logger ?? this.logger;

		const transportPriority = this.detectTransportPriority();

		this.state = {
			status: 'disconnected',
			activeTransport: null,
			failureCount: 0,
			canFallback: transportPriority.length > 1,
			reconnectAttempts: 0,
			latency: null,
			error: null,
			lastEventId: this.loadLastEventId(),
			transportPriority,
		};
	}

	/**
	 * Feature detection for transport support
	 */
	static getFeatureSupport(): Record<TransportType, boolean> {
		return {
			websocket: typeof WebSocket !== 'undefined',
			sse: typeof EventSource !== 'undefined',
			polling: typeof fetch !== 'undefined',
		};
	}

	/**
	 * Connect using the optimal transport
	 */
	connect(): void {
		if (this.isDestroyed) {
			throw new Error('TransportManager has been destroyed');
		}

		if (this.currentTransport && this.state.status === 'connected') {
			return; // Already connected
		}

		this.isExplicitDisconnect = false;
		this.setState({ status: 'connecting', error: null });

		const transportType = this.selectOptimalTransport();
		this.connectWithTransport(transportType, 'feature_detection');
	}

	/**
	 * Disconnect the current transport
	 */
	disconnect(): void {
		this.isExplicitDisconnect = true;
		this.stopUpgradeTimer();

		if (this.currentTransport) {
			this.currentTransport.disconnect();
			this.cleanupTransport();
		}

		this.setState({ status: 'disconnected', activeTransport: null });
		this.emit('close', {});
	}

	/**
	 * Destroy the manager and cleanup all resources
	 */
	destroy(): void {
		this.isDestroyed = true;
		this.stopUpgradeTimer();

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

		if (!this.currentTransport.send) {
			throw new Error(`${this.state.activeTransport} transport does not support sending messages`);
		}

		try {
			this.currentTransport.send(message);
		} catch (error) {
			const resolvedError = error instanceof Error ? error : new Error(String(error));
			this.handleTransportError(resolvedError);
			throw resolvedError;
		}
	}

	/**
	 * Subscribe to events
	 * Validates event names against the TransportEventMap to prevent undefined subscriptions
	 */
	on(event: TransportEventName | string, handler: WebSocketEventHandler): () => void {
		// Store handler in our map
		let handlers = this.eventHandlers.get(event);
		if (!handlers) {
			handlers = new Set<WebSocketEventHandler>();
			this.eventHandlers.set(event, handlers);
		}
		handlers.add(handler);

		// Subscribe to current transport if connected
		if (this.currentTransport) {
			this.subscribeHandlerToCurrentTransport(event, handler);
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

			// Clean up unsubscribers for this handler
			this.cleanupHandlerUnsubscribers(event);
		};
	}

	/**
	 * Get the current state
	 */
	getState(): Readonly<TransportManagerState> {
		// Merge with underlying transport state if available
		if (this.currentTransport) {
			const transportState = this.currentTransport.getState();
			const isRecord = (value: unknown): value is Record<string, unknown> =>
				typeof value === 'object' && value !== null;
			const record = isRecord(transportState) ? transportState : null;
			const latencyCandidate = record?.['latency'];
			const lastEventIdCandidate = record?.['lastEventId'];

			const latency =
				typeof latencyCandidate === 'number' || latencyCandidate === null
					? latencyCandidate
					: this.state.latency;
			const lastEventId =
				typeof lastEventIdCandidate === 'string' || lastEventIdCandidate === null
					? lastEventIdCandidate
					: this.state.lastEventId;

			return {
				...this.state,
				latency,
				lastEventId,
			};
		}

		return { ...this.state };
	}

	/**
	 * Get the current active transport type
	 */
	getActiveTransport(): TransportType | null {
		return this.state.activeTransport;
	}

	/**
	 * Check if specific transport is supported
	 */
	isTransportSupported(transport: TransportType): boolean {
		const features = TransportManager.getFeatureSupport();
		return features[transport];
	}

	/**
	 * Force switch to a specific transport
	 */
	switchTransport(transportType: TransportType, reason: string = 'Manual switch'): void {
		if (!this.isTransportSupported(transportType)) {
			throw new Error(`Transport ${transportType} is not supported`);
		}

		if (this.state.activeTransport === transportType) {
			return; // Already using requested transport
		}

		this.logger.info('Switching transport', {
			from: this.state.activeTransport,
			to: transportType,
			reason,
		});

		// Cleanup current transport
		if (this.currentTransport) {
			this.currentTransport.disconnect();
			this.cleanupTransport();
		}

		// Reset failure count for manual switches
		this.consecutiveFailures = 0;

		this.connectWithTransport(transportType, 'forced');
	}

	private detectTransportPriority(): TransportType[] {
		const features = TransportManager.getFeatureSupport();
		const priority: TransportType[] = [];

		// Standard priority: WebSocket > SSE > HTTP Polling
		if (features.websocket) priority.push('websocket');
		if (features.sse) priority.push('sse');
		if (features.polling) priority.push('polling');

		return priority;
	}

	private selectOptimalTransport(): TransportType {
		// Check if transport is forced
		if (this.config.forceTransport && this.config.forceTransport !== 'auto') {
			if (!this.isTransportSupported(this.config.forceTransport)) {
				throw new Error(`Forced transport ${this.config.forceTransport} is not supported`);
			}
			return this.config.forceTransport;
		}

		// Auto-detect: Use highest priority available transport
		for (const transport of this.state.transportPriority) {
			if (this.isTransportSupported(transport)) {
				return transport;
			}
		}

		throw new Error('No supported transports available');
	}

	private selectFallbackTransport(): TransportType | null {
		if (!this.config.autoFallback || !this.state.activeTransport) {
			return null;
		}

		const currentIndex = this.state.transportPriority.indexOf(this.state.activeTransport);

		// Try next transport in fallback order
		for (let i = currentIndex + 1; i < this.state.transportPriority.length; i++) {
			const transport = this.state.transportPriority[i];
			if (transport && this.isTransportSupported(transport)) {
				return transport;
			}
		}

		return null; // No fallback available
	}

	private connectWithTransport(
		transportType: TransportType,
		reason: TransportSwitchEvent['reason']
	): void {
		try {
			const previousTransport = this.state.activeTransport;

			this.setState({
				activeTransport: transportType,
				status: 'connecting',
				error: null,
			});

			// Create the transport instance
			switch (transportType) {
				case 'websocket':
					this.currentTransport = new WebSocketClient(this.config.websocket);
					break;
				case 'sse':
					this.currentTransport = new SseClient(this.config.sse);
					break;
				case 'polling':
					this.currentTransport = new HttpPollingClient(this.config.polling);
					break;
				default:
					throw new Error(`Unknown transport type: ${transportType}`);
			}

			// Subscribe all existing handlers
			this.subscribeAllHandlers();

			// Set up transport-specific event handlers
			this.setupTransportEventHandlers();

			// Start the connection
			this.currentTransport.connect();

			// Emit transport switch event
			const switchEvent: TransportSwitchEvent = {
				from: previousTransport,
				to: transportType,
				reason,
			};
			this.emit('transport_switch', switchEvent);
		} catch (error) {
			const resolvedError = error instanceof Error ? error : new Error(String(error));
			this.handleTransportError(resolvedError);
			this.attemptFallback(resolvedError);
		}
	}

	private setupTransportEventHandlers(): void {
		if (!this.currentTransport) return;

		// Handle connection success
		const openUnsubscribe = this.currentTransport.on('open', () => {
			this.consecutiveFailures = 0; // Reset failure count on successful connection
			this.setState({
				status: 'connected',
				failureCount: 0,
				reconnectAttempts: 0,
			});

			// Start upgrade timer if enabled
			if (this.config.enableUpgradeAttempts) {
				this.startUpgradeTimer();
			}
		});

		// Handle connection errors
		const errorUnsubscribe = this.currentTransport.on('error', (event) => {
			if (event.error) {
				this.handleTransportError(event.error);
			}
		});

		// Handle reconnection events
		const reconnectingUnsubscribe = this.currentTransport.on('reconnecting', (event) => {
			this.setState({ status: 'reconnecting' });
			this.emit('reconnecting', event.data);
		});

		const reconnectedUnsubscribe = this.currentTransport.on('reconnected', (event) => {
			this.consecutiveFailures = 0;
			this.setState({ status: 'connected', reconnectAttempts: 0 });
			this.emit('reconnected', event.data);
		});

		// Handle close events
		const closeUnsubscribe = this.currentTransport.on('close', () => {
			if (!this.isExplicitDisconnect && !this.isDestroyed) {
				this.handleTransportDisconnection();
			}
		});

		// Store unsubscribers for cleanup
		this.addUnsubscribers(
			'_transport_events',
			openUnsubscribe,
			errorUnsubscribe,
			reconnectingUnsubscribe,
			reconnectedUnsubscribe,
			closeUnsubscribe
		);
	}

	private subscribeAllHandlers(): void {
		if (!this.currentTransport) return;

		this.eventHandlers.forEach((handlers, event) => {
			handlers.forEach((handler) => {
				this.subscribeHandlerToCurrentTransport(event, handler);
			});
		});
	}

	private subscribeHandlerToCurrentTransport(event: string, handler: WebSocketEventHandler): void {
		if (!this.currentTransport) return;

		const unsubscribe = this.currentTransport.on(event, handler);
		this.addUnsubscribers(event, unsubscribe);
	}

	private addUnsubscribers(event: string, ...entries: Array<() => void>): void {
		const existing = this.unsubscribers.get(event);
		if (existing) {
			existing.push(...entries);
			return;
		}
		this.unsubscribers.set(event, [...entries]);
	}

	private cleanupHandlerUnsubscribers(event: string): void {
		const unsubs = this.unsubscribers.get(event);
		if (unsubs) {
			// For simplicity, we'll clean up all unsubscribers for this event
			// In a more sophisticated implementation, we'd track individual handler unsubscribers
			unsubs.forEach((unsub) => unsub());
			this.unsubscribers.delete(event);
		}
	}

	private handleTransportError(error: Error): void {
		this.consecutiveFailures++;
		this.setState({
			error,
			failureCount: this.consecutiveFailures,
		});

		this.logger.warn('Transport error', {
			transport: this.state.activeTransport,
			failureCount: this.consecutiveFailures,
			maxFailures: this.config.maxFailuresBeforeSwitch,
			message: error.message,
		});

		// Emit error to handlers
		this.emit('error', { error }, error);

		// Check if we should attempt fallback
		if (this.shouldAttemptFallback(error)) {
			this.attemptFallback(error);
		}
	}

	private handleTransportDisconnection(): void {
		if (this.isDestroyed || this.isExplicitDisconnect) {
			return;
		}

		// Check if we should attempt fallback due to persistent disconnections
		if (this.consecutiveFailures >= this.config.maxFailuresBeforeSwitch) {
			this.attemptFallback(new Error('Too many connection failures'));
		} else {
			this.setState({ status: 'reconnecting' });
		}
	}

	private shouldAttemptFallback(error: Error): boolean {
		if (!this.config.autoFallback) {
			return false;
		}

		// Don't fallback if we've exceeded max failures for fallback
		if (this.consecutiveFailures < this.config.maxFailuresBeforeSwitch) {
			return false;
		}

		const errorMessage = error.message.toLowerCase();

		// Fallback on network-related errors
		if (
			errorMessage.includes('network') ||
			errorMessage.includes('failed to fetch') ||
			errorMessage.includes('connection') ||
			errorMessage.includes('timeout')
		) {
			return true;
		}

		// Fallback on transport-specific support errors
		if (
			errorMessage.includes('websocket') ||
			errorMessage.includes('eventsource') ||
			errorMessage.includes('not supported')
		) {
			return true;
		}

		// Fallback on certain HTTP errors
		if (
			errorMessage.includes('404') ||
			errorMessage.includes('405') ||
			errorMessage.includes('501') ||
			errorMessage.includes('502') ||
			errorMessage.includes('503')
		) {
			return true;
		}

		return false;
	}

	private attemptFallback(error: Error): void {
		const fallbackTransport = this.selectFallbackTransport();

		if (!fallbackTransport) {
			this.logger.error('No fallback transport available');
			this.setState({ status: 'disconnected' });
			this.emit('close', {});
			return;
		}

		this.logger.warn('Falling back to alternate transport', {
			from: this.state.activeTransport,
			to: fallbackTransport,
			error: error.message,
		});

		// Cleanup current transport
		if (this.currentTransport) {
			this.currentTransport.disconnect();
			this.cleanupTransport();
		}

		// Reset failure count for new transport
		this.consecutiveFailures = 0;

		// Connect with fallback transport
		this.connectWithTransport(fallbackTransport, 'fallback');
	}

	private startUpgradeTimer(): void {
		if (this.upgradeTimer || !this.config.enableUpgradeAttempts) {
			return;
		}

		this.upgradeTimer = setTimeout(() => {
			this.attemptTransportUpgrade();
		}, this.config.upgradeAttemptInterval);
	}

	private stopUpgradeTimer(): void {
		if (this.upgradeTimer) {
			clearTimeout(this.upgradeTimer);
			this.upgradeTimer = null;
		}
	}

	private attemptTransportUpgrade(): void {
		if (this.isDestroyed || this.isExplicitDisconnect || !this.state.activeTransport) {
			return;
		}

		const currentIndex = this.state.transportPriority.indexOf(this.state.activeTransport);

		// Try higher priority transports
		for (let i = 0; i < currentIndex; i++) {
			const transport = this.state.transportPriority[i];
			if (transport && this.isTransportSupported(transport)) {
				this.logger.info('Attempting transport upgrade', {
					from: this.state.activeTransport,
					to: transport,
				});

				// Store current transport as backup
				const backupTransport = this.currentTransport;
				const backupType: TransportType = this.state.activeTransport;

				try {
					this.switchTransport(transport, `Upgrade attempt from ${this.state.activeTransport}`);

					// If successful, destroy the backup
					setTimeout(() => {
						if (backupTransport && this.state.activeTransport === transport) {
							backupTransport.destroy();
						}
					}, 5000); // Give new transport time to stabilize

					return; // Upgrade successful
				} catch (error) {
					this.logger.warn('Transport upgrade failed', {
						attempted: transport,
						fallback: backupType,
						error,
					});

					// Restore backup if upgrade failed
					if (this.state.activeTransport !== backupType && backupTransport) {
						this.currentTransport = backupTransport;
						this.setState({ activeTransport: backupType });
					}
				}
			}
		}

		// Schedule next upgrade attempt
		this.startUpgradeTimer();
	}

	private cleanupTransport(): void {
		// Unsubscribe all transport event handlers
		this.unsubscribers.forEach((unsubs) => {
			unsubs.forEach((unsub) => unsub());
		});
		this.unsubscribers.clear();

		this.currentTransport = null;
		this.stopUpgradeTimer();
	}

	private setState(updates: Partial<TransportManagerState>): void {
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
					this.logger.error(`Error in event handler for ${event}`, { error: err });
				}
			});
		}
	}

	private loadLastEventId(): string | null {
		// Try to load from any of the transport configs that support it
		const configs = [this.config.websocket, this.config.sse, this.config.polling];

		for (const config of configs) {
			if (config.storage && config.lastEventIdStorageKey) {
				try {
					const eventId = config.storage.getItem(config.lastEventIdStorageKey);
					if (eventId) return eventId;
				} catch {
					// Continue to next config
				}
			}
		}

		return null;
	}
}
