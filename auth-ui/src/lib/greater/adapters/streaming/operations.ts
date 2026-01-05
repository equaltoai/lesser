/**
 * Streaming operations handlers for real-time updates
 * Manages streaming deletes, edits, and other real-time operations
 */

import type { TransportManager } from '../TransportManager.js';
import type {
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit,
} from '../models/unified.js';

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null;

const isUnifiedAccountRecord = (value: unknown): value is UnifiedAccount => {
	if (!isRecord(value)) {
		return false;
	}

	const id = value['id'];
	const username = value['username'];

	return typeof id === 'string' && typeof username === 'string';
};

const isUnifiedStatusRecord = (value: unknown): value is UnifiedStatus => {
	if (!isRecord(value)) {
		return false;
	}

	const id = value['id'];
	const content = value['content'];
	const account = value['account'];

	return typeof id === 'string' && typeof content === 'string' && isUnifiedAccountRecord(account);
};

const isUnifiedNotificationRecord = (value: unknown): value is UnifiedNotification => {
	if (!isRecord(value)) {
		return false;
	}

	const id = value['id'];
	const type = value['type'];
	const account = value['account'];

	return typeof id === 'string' && typeof type === 'string' && isUnifiedAccountRecord(account);
};

// Streaming operation types
export type StreamingOperation = StreamingUpdate | StreamingDelete | StreamingEdit;

export interface StreamingOperationHandler {
	/** Handle streaming operation */
	handle(operation: StreamingOperation): Promise<void>;
	/** Operation type this handler supports */
	type: string;
}

export interface StreamingState {
	/** Whether streaming is active */
	isStreaming: boolean;
	/** Connected streams */
	activeStreams: Set<string>;
	/** Last event timestamp */
	lastEventTime: number | null;
	/** Error state */
	error: Error | null;
	/** Reconnection attempts */
	reconnectAttempts: number;
}

export interface StreamingConfig {
	/** Transport manager for streaming */
	transportManager: TransportManager;
	/** Debounce time for batching operations in ms */
	debounceMs?: number;
	/** Maximum queue size before dropping old operations */
	maxQueueSize?: number;
	/** Enable operation deduplication */
	enableDeduplication?: boolean;
	/** Deduplication window in ms */
	deduplicationWindowMs?: number;
}

/**
 * Main streaming operations manager
 */
export class StreamingOperationsManager {
	private handlers = new Map<string, StreamingOperationHandler>();
	private operationQueue: StreamingOperation[] = [];
	private processingQueue = false;
	private deduplicationCache = new Map<string, number>();
	private state: StreamingState = {
		isStreaming: false,
		activeStreams: new Set(),
		lastEventTime: null,
		error: null,
		reconnectAttempts: 0,
	};

	constructor(private config: StreamingConfig) {
		this.setupTransportListeners();
	}

	/**
	 * Register a streaming operation handler
	 */
	registerHandler(handler: StreamingOperationHandler): void {
		this.handlers.set(handler.type, handler);
	}

	/**
	 * Unregister a streaming operation handler
	 */
	unregisterHandler(type: string): void {
		this.handlers.delete(type);
	}

	/**
	 * Start streaming operations
	 */
	async startStreaming(): Promise<void> {
		try {
			this.config.transportManager.connect();
			this.state.isStreaming = true;
			this.state.error = null;
			this.state.reconnectAttempts = 0;
		} catch (error) {
			this.state.error = error instanceof Error ? error : new Error('Failed to start streaming');
			throw this.state.error;
		}
	}

	/**
	 * Stop streaming operations
	 */
	async stopStreaming(): Promise<void> {
		this.config.transportManager.disconnect();
		this.state.isStreaming = false;
		this.state.activeStreams.clear();

		// Process remaining queued operations
		if (this.operationQueue.length > 0) {
			await this.processQueue();
		}
	}

	/**
	 * Get current streaming state
	 */
	getState(): StreamingState {
		return { ...this.state };
	}

	/**
	 * Process streaming operation
	 */
	async processOperation(operation: StreamingOperation): Promise<void> {
		// Add to queue for batched processing
		if (this.shouldDeduplicate(operation)) {
			return;
		}

		this.operationQueue.push(operation);

		// Trim queue if too large
		if (this.config.maxQueueSize && this.operationQueue.length > this.config.maxQueueSize) {
			this.operationQueue = this.operationQueue.slice(-this.config.maxQueueSize);
		}

		// Trigger debounced processing
		if (!this.processingQueue) {
			await this.scheduleQueueProcessing();
		}
	}

	/**
	 * Check if operation should be deduplicated
	 */
	private shouldDeduplicate(operation: StreamingOperation): boolean {
		if (!this.config.enableDeduplication) {
			return false;
		}

		const operationKey = this.getOperationKey(operation);
		const now = Date.now();
		const windowMs = this.config.deduplicationWindowMs || 1000;

		const lastTime = this.deduplicationCache.get(operationKey);
		if (lastTime && now - lastTime < windowMs) {
			return true;
		}

		this.deduplicationCache.set(operationKey, now);

		// Clean up old cache entries
		const entries = Array.from(this.deduplicationCache.entries());
		for (const [key, time] of entries) {
			if (now - time > windowMs * 2) {
				this.deduplicationCache.delete(key);
			}
		}

		return false;
	}

	/**
	 * Generate deduplication key for operation
	 */
	private getOperationKey(operation: StreamingOperation): string {
		if ('id' in operation && 'editType' in operation) {
			return `edit-${operation.id}`;
		}
		if ('itemType' in operation && 'id' in operation) {
			return `delete-${operation.itemType}-${operation.id}`;
		}
		if ('type' in operation && 'payload' in operation) {
			const rawType = operation['type'];
			const operationType = typeof rawType === 'string' && rawType.length > 0 ? rawType : 'unknown';
			const payload = operation['payload'];

			if (isRecord(payload)) {
				const payloadId = payload['id'];
				if (typeof payloadId === 'string') {
					return `${operationType}-${payloadId}`;
				}
			}

			const timestamp =
				typeof operation['timestamp'] === 'number' ? operation['timestamp'] : Date.now();
			return `${operationType}-${timestamp}`;
		}
		const timestamp =
			typeof operation['timestamp'] === 'number' ? operation['timestamp'] : Date.now();
		return `unknown-${timestamp}`;
	}

	/**
	 * Schedule queue processing with debouncing
	 */
	private async scheduleQueueProcessing(): Promise<void> {
		if (this.processingQueue) {
			return;
		}

		const debounceMs = this.config.debounceMs || 100;

		await new Promise((resolve) => setTimeout(resolve, debounceMs));
		await this.processQueue();
	}

	/**
	 * Process the operation queue
	 */
	private async processQueue(): Promise<void> {
		if (this.processingQueue || this.operationQueue.length === 0) {
			return;
		}

		this.processingQueue = true;
		const operations = this.operationQueue.splice(0);

		try {
			// Group operations by type for batched processing
			const operationGroups = new Map<string, StreamingOperation[]>();

			for (const operation of operations) {
				const type = this.getOperationType(operation);
				let group = operationGroups.get(type);
				if (!group) {
					group = [];
					operationGroups.set(type, group);
				}
				group.push(operation);
			}

			// Process each group
			const operationGroupEntries = Array.from(operationGroups.entries());
			for (const [type, groupOperations] of operationGroupEntries) {
				const handler = this.handlers.get(type);
				if (handler) {
					for (const operation of groupOperations) {
						try {
							await handler.handle(operation);
							if ('timestamp' in operation) {
								const timestamp =
									typeof operation['timestamp'] === 'number' ? operation['timestamp'] : Date.now();
								this.state.lastEventTime = timestamp;
							}
						} catch (error) {
							console.error(`Failed to handle streaming operation ${type}:`, error);
						}
					}
				}
			}
		} finally {
			this.processingQueue = false;
		}
	}

	/**
	 * Get operation type for handler routing
	 */
	private getOperationType(operation: StreamingOperation): string {
		if ('itemType' in operation) {
			return 'delete';
		}
		if ('editType' in operation) {
			return 'edit';
		}
		if ('type' in operation) {
			const rawType = operation['type'];
			return typeof rawType === 'string' && rawType.length > 0 ? rawType : 'update';
		}
		return 'update';
	}

	/**
	 * Setup transport event listeners
	 */
	private setupTransportListeners(): void {
		this.config.transportManager.on('message', (event) => {
			this.handleTransportMessage(event.data);
		});

		this.config.transportManager.on('error', (event) => {
			this.state.error = event.error || new Error('Transport error');
		});

		this.config.transportManager.on('reconnecting', () => {
			this.state.reconnectAttempts++;
		});

		this.config.transportManager.on('reconnected', () => {
			this.state.error = null;
			this.state.reconnectAttempts = 0;
		});
	}

	/**
	 * Handle incoming transport message
	 */
	private handleTransportMessage(data: unknown): void {
		try {
			// The data should already be processed by API-specific mappers
			if (this.isStreamingOperation(data)) {
				this.processOperation(data);
			}
		} catch (error) {
			console.error('Failed to process transport message:', error);
		}
	}

	/**
	 * Type guard for streaming operations
	 */
	private isStreamingOperation(data: unknown): data is StreamingOperation {
		if (!isRecord(data)) {
			return false;
		}

		return (
			('type' in data && 'payload' in data) ||
			('id' in data && 'itemType' in data) ||
			('id' in data && 'editType' in data)
		);
	}

	/**
	 * Clean up resources
	 */
	destroy(): void {
		this.stopStreaming();
		this.handlers.clear();
		this.operationQueue = [];
		this.deduplicationCache.clear();
	}
}

/**
 * Timeline streaming handler for status updates
 */
export class TimelineStreamingHandler implements StreamingOperationHandler {
	type = 'status';

	constructor(
		private onStatusUpdate: (status: UnifiedStatus) => void,
		private onStatusDelete: (statusId: string) => void,
		private onStatusEdit: (status: UnifiedStatus) => void
	) {}

	async handle(operation: StreamingOperation): Promise<void> {
		if ('itemType' in operation && operation.itemType === 'status') {
			this.onStatusDelete(operation.id);
		} else if ('editType' in operation && 'data' in operation) {
			if (isUnifiedStatusRecord(operation.data)) {
				this.onStatusEdit(operation.data);
			}
		} else if ('payload' in operation) {
			if (isUnifiedStatusRecord(operation.payload)) {
				this.onStatusUpdate(operation.payload);
			}
		}
	}
}

/**
 * Notification streaming handler
 */
export class NotificationStreamingHandler implements StreamingOperationHandler {
	type = 'notification';

	constructor(
		private onNotificationUpdate: (notification: UnifiedNotification) => void,
		private onNotificationDelete: (notificationId: string) => void
	) {}

	async handle(operation: StreamingOperation): Promise<void> {
		if ('itemType' in operation && operation.itemType === 'notification') {
			this.onNotificationDelete(operation.id);
		} else if ('payload' in operation) {
			if (isUnifiedNotificationRecord(operation.payload)) {
				this.onNotificationUpdate(operation.payload);
			}
		}
	}
}

/**
 * Account streaming handler for user updates
 */
export class AccountStreamingHandler implements StreamingOperationHandler {
	type = 'account';

	constructor(
		private onAccountUpdate: (account: UnifiedAccount) => void,
		private onAccountDelete: (accountId: string) => void
	) {}

	async handle(operation: StreamingOperation): Promise<void> {
		if ('itemType' in operation && operation.itemType === 'account') {
			this.onAccountDelete(operation.id);
		} else if ('payload' in operation) {
			if (isUnifiedAccountRecord(operation.payload)) {
				this.onAccountUpdate(operation.payload);
			}
		} else if ('editType' in operation && 'data' in operation) {
			if (isUnifiedAccountRecord(operation.data)) {
				this.onAccountUpdate(operation.data);
			}
		}
	}
}

/**
 * Delete operation handler
 */
export class DeleteStreamingHandler implements StreamingOperationHandler {
	type = 'delete';

	constructor(private onDelete: (deleteOperation: StreamingDelete) => void) {}

	async handle(operation: StreamingOperation): Promise<void> {
		if ('itemType' in operation) {
			this.onDelete(operation as StreamingDelete);
		}
	}
}

/**
 * Edit operation handler
 */
export class EditStreamingHandler implements StreamingOperationHandler {
	type = 'edit';

	constructor(private onEdit: (editOperation: StreamingEdit) => void) {}

	async handle(operation: StreamingOperation): Promise<void> {
		if ('editType' in operation) {
			this.onEdit(operation as StreamingEdit);
		}
	}
}

/**
 * State consistency manager for streaming operations
 */
export class StreamingStateManager {
	private statusCache = new Map<string, UnifiedStatus>();
	private accountCache = new Map<string, UnifiedAccount>();
	private notificationCache = new Map<string, UnifiedNotification>();

	/**
	 * Apply streaming operation with state consistency checks
	 */
	async applyOperation(
		operation: StreamingOperation
	): Promise<{ applied: boolean; conflicts: string[] }> {
		const conflicts: string[] = [];
		let applied = false;

		try {
			if ('itemType' in operation) {
				// Handle delete operation
				applied = this.applyDelete(operation);
			} else if ('editType' in operation && 'data' in operation) {
				// Handle edit operation
				const result = this.applyEdit(operation);
				applied = result.applied;
				conflicts.push(...result.conflicts);
			} else if ('payload' in operation) {
				// Handle update operation
				applied = this.applyUpdate(operation);
			}
		} catch (error) {
			console.error('Failed to apply streaming operation:', error);
		}

		return { applied, conflicts };
	}

	/**
	 * Apply delete operation
	 */
	private applyDelete(operation: StreamingDelete): boolean {
		switch (operation.itemType) {
			case 'status':
				return this.statusCache.delete(operation.id);
			case 'account':
				return this.accountCache.delete(operation.id);
			case 'notification':
				return this.notificationCache.delete(operation.id);
			default:
				return false;
		}
	}

	/**
	 * Apply edit operation with conflict detection
	 */
	private applyEdit(operation: StreamingEdit): { applied: boolean; conflicts: string[] } {
		const conflicts: string[] = [];

		if (isUnifiedStatusRecord(operation.data)) {
			const existing = this.statusCache.get(operation.id);
			if (existing) {
				// Check for edit conflicts
				if (existing.editedAt && operation.data.editedAt) {
					const existingTime = new Date(existing.editedAt).getTime();
					const newTime = new Date(operation.data.editedAt).getTime();
					if (newTime < existingTime) {
						conflicts.push(`Edit operation is older than existing edit: ${operation.id}`);
						return { applied: false, conflicts };
					}
				}
			}
			this.statusCache.set(operation.id, operation.data);
			return { applied: true, conflicts };
		}

		if (isUnifiedAccountRecord(operation.data)) {
			this.accountCache.set(operation.id, operation.data);
			return { applied: true, conflicts };
		}

		if (isUnifiedNotificationRecord(operation.data)) {
			this.notificationCache.set(operation.id, operation.data);
			return { applied: true, conflicts };
		}

		return { applied: false, conflicts: ['Unknown data type in edit operation'] };
	}

	/**
	 * Apply update operation
	 */
	private applyUpdate(operation: StreamingUpdate): boolean {
		if (isUnifiedStatusRecord(operation.payload)) {
			this.statusCache.set(operation.payload.id, operation.payload);
			return true;
		}

		if (isUnifiedAccountRecord(operation.payload)) {
			this.accountCache.set(operation.payload.id, operation.payload);
			return true;
		}

		if (isUnifiedNotificationRecord(operation.payload)) {
			this.notificationCache.set(operation.payload.id, operation.payload);
			return true;
		}

		return false;
	}

	/**
	 * Get cached item by ID and type
	 */
	getCachedItem(
		id: string,
		type: 'status' | 'account' | 'notification'
	): UnifiedStatus | UnifiedAccount | UnifiedNotification | undefined {
		switch (type) {
			case 'status':
				return this.statusCache.get(id);
			case 'account':
				return this.accountCache.get(id);
			case 'notification':
				return this.notificationCache.get(id);
		}
	}

	/**
	 * Clear all cached data
	 */
	clearCache(): void {
		this.statusCache.clear();
		this.accountCache.clear();
		this.notificationCache.clear();
	}

	/**
	 * Get cache statistics
	 */
	getCacheStats(): { statuses: number; accounts: number; notifications: number } {
		return {
			statuses: this.statusCache.size,
			accounts: this.accountCache.size,
			notifications: this.notificationCache.size,
		};
	}
}
