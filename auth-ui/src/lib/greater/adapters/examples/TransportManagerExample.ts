/**
 * Example demonstrating how to use the TransportManager for unified event handling
 * with automatic fallback across WebSocket → SSE → HTTP Polling
 */

import { TransportManager } from '../TransportManager';
import type {
	TransportManagerConfig,
	TransportManagerState,
	TransportSwitchEvent,
	TransportType,
	TransportLogger,
} from '../types';

type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
	level: LogLevel;
	message: string;
	context?: unknown;
	timestamp: number;
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null;

const getNumberField = (value: unknown, field: string): number | undefined => {
	if (!isRecord(value)) {
		return undefined;
	}

	const candidate = value[field];
	return typeof candidate === 'number' ? candidate : undefined;
};

const isTransportSwitchEvent = (value: unknown): value is TransportSwitchEvent => {
	if (!isRecord(value)) {
		return false;
	}

	const from = value['from'];
	const to = value['to'];
	const reason = value['reason'];

	return (
		(from === null || typeof from === 'string') &&
		typeof to === 'string' &&
		typeof reason === 'string'
	);
};

interface ExampleStatus {
	connected: boolean;
	activeTransport: TransportType | null;
	state: TransportManagerState;
	featureSupport: Record<TransportType, boolean>;
}

// Example configuration for all three transport types
const config: TransportManagerConfig = {
	websocket: {
		url: 'ws://localhost:8080/ws',
		authToken: 'your-auth-token',
		heartbeatInterval: 30000,
		heartbeatTimeout: 60000,
	},
	sse: {
		url: 'http://localhost:8080/events',
		authToken: 'your-auth-token',
		heartbeatInterval: 30000,
		heartbeatTimeout: 60000,
		withCredentials: false,
		headers: {
			'X-Client-Version': '1.0.0',
		},
	},
	polling: {
		url: 'http://localhost:8080/api',
		pollingInterval: 5000,
		authToken: 'your-auth-token',
		withCredentials: false,
		headers: {
			'X-Client-Version': '1.0.0',
		},
		requestTimeout: 30000,
	},

	// Configuration options
	autoFallback: true, // Enable automatic fallback
	forceTransport: 'auto', // Let the manager choose optimal transport
	maxFailuresBeforeSwitch: 3, // Switch after 3 consecutive failures
	enableUpgradeAttempts: true, // Try to upgrade to better transports
	upgradeAttemptInterval: 300000, // Try upgrade every 5 minutes
};

export class ExampleApp {
	private readonly transportManager: TransportManager;
	private readonly logs: LogEntry[] = [];
	private readonly logger: TransportLogger;
	private isConnected = false;

	constructor(managerConfig: TransportManagerConfig = config) {
		this.logger = {
			debug: (message, context) => this.log('debug', message, context),
			info: (message, context) => this.log('info', message, context),
			warn: (message, context) => this.log('warn', message, context),
			error: (message, error) => this.log('error', message, error),
		};

		this.transportManager = new TransportManager({
			...managerConfig,
			logger: managerConfig.logger ?? this.logger,
		});

		this.setupEventHandlers();
	}

	private setupEventHandlers(): void {
		this.transportManager.on('open', () => {
			this.isConnected = true;
			this.log('info', 'Transport connected', {
				transport: this.transportManager.getActiveTransport(),
			});
		});

		this.transportManager.on('close', () => {
			this.isConnected = false;
			this.log('warn', 'Transport disconnected');
		});

		this.transportManager.on('error', (event) => {
			this.log('error', 'Transport error', event.error ?? event.data);
		});

		this.transportManager.on('transport_switch', (event) => {
			if (isTransportSwitchEvent(event.data)) {
				this.log('info', 'Transport switched', event.data);
			} else {
				this.log('warn', 'Transport switch event missing payload');
			}
		});

		this.transportManager.on('reconnecting', (event) => {
			const attempt = getNumberField(event.data, 'attempt');
			const delay = getNumberField(event.data, 'delay');
			this.log('info', 'Reconnecting transport', { attempt, delay });
		});

		this.transportManager.on('reconnected', () => {
			this.log('info', 'Transport reconnected');
		});

		this.transportManager.on('message', (event) => {
			this.log('info', 'Received message', event.data);
		});

		this.transportManager.on('user_joined', (event) => {
			this.log('info', 'User joined', event.data);
		});

		this.transportManager.on('user_left', (event) => {
			this.log('info', 'User left', event.data);
		});

		this.transportManager.on('chat_message', (event) => {
			this.log('info', 'Chat message', event.data);
		});

		this.transportManager.on('latency', (event) => {
			const latency = getNumberField(event.data, 'latency');
			if (latency !== undefined) {
				this.log('debug', 'Latency sample', {
					latency,
					transport: this.transportManager.getActiveTransport(),
				});
			}
		});
	}

	async connect(): Promise<void> {
		try {
			this.log('info', 'Starting connection', {
				featureSupport: TransportManager.getFeatureSupport(),
			});
			this.transportManager.connect();
		} catch (error) {
			this.log('error', 'Failed to connect', error);
			throw error;
		}
	}

	disconnect(): void {
		this.log('info', 'Disconnecting transport');
		this.transportManager.disconnect();
	}

	sendMessage(type: string, data: Record<string, unknown>): void {
		if (!this.isConnected) {
			throw new Error('Not connected');
		}

		const message = {
			type,
			data,
			timestamp: Date.now(),
		};

		try {
			this.transportManager.send(message);
			this.log('info', 'Sent message', {
				type,
				transport: this.transportManager.getActiveTransport(),
			});
		} catch (error) {
			this.log('error', 'Failed to send message', error);
			throw error;
		}
	}

	switchTransport(transportType: TransportType): void {
		try {
			this.transportManager.switchTransport(transportType, 'Manual switch by user');
			this.log('info', 'Manually switched transport', { transportType });
		} catch (error) {
			this.log('error', 'Failed to switch transport', error);
			throw error;
		}
	}

	getStatus(): ExampleStatus {
		return {
			connected: this.isConnected,
			activeTransport: this.transportManager.getActiveTransport(),
			state: this.transportManager.getState(),
			featureSupport: TransportManager.getFeatureSupport(),
		};
	}

	captureStatus(label: string): void {
		this.log('info', label, this.getStatus());
	}

	getLogs(): LogEntry[] {
		return [...this.logs];
	}

	recordError(message: string, error: unknown): void {
		this.log('error', message, error);
	}

	destroy(): void {
		this.log('info', 'Cleaning up resources');
		this.transportManager.destroy();
	}

	private log(level: LogLevel, message: string, context?: unknown): void {
		this.logs.push({
			level,
			message,
			context,
			timestamp: Date.now(),
		});
	}
}

const delay = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

export async function runExample(): Promise<LogEntry[]> {
	const app = new ExampleApp();

	try {
		await app.connect();
		await delay(1000);

		app.sendMessage('chat_message', { text: 'Hello World!', user: 'john' });
		app.sendMessage('user_status', { user: 'john', status: 'online' });

		app.captureStatus('Current status snapshot');

		const status = app.getStatus();
		if (status.featureSupport.polling) {
			app.switchTransport('polling');
			await delay(1000);
			app.sendMessage('test_message', { test: 'polling transport' });
		}

		await delay(5000);
	} catch (error) {
		app.recordError('Example error', error);
	} finally {
		app.destroy();
	}

	return app.getLogs();
}
