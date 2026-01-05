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
export declare class StreamingOperationsManager {
	private config;
	private handlers;
	private operationQueue;
	private processingQueue;
	private deduplicationCache;
	private state;
	constructor(config: StreamingConfig);
	/**
	 * Register a streaming operation handler
	 */
	registerHandler(handler: StreamingOperationHandler): void;
	/**
	 * Unregister a streaming operation handler
	 */
	unregisterHandler(type: string): void;
	/**
	 * Start streaming operations
	 */
	startStreaming(): Promise<void>;
	/**
	 * Stop streaming operations
	 */
	stopStreaming(): Promise<void>;
	/**
	 * Get current streaming state
	 */
	getState(): StreamingState;
	/**
	 * Process streaming operation
	 */
	processOperation(operation: StreamingOperation): Promise<void>;
	/**
	 * Check if operation should be deduplicated
	 */
	private shouldDeduplicate;
	/**
	 * Generate deduplication key for operation
	 */
	private getOperationKey;
	/**
	 * Schedule queue processing with debouncing
	 */
	private scheduleQueueProcessing;
	/**
	 * Process the operation queue
	 */
	private processQueue;
	/**
	 * Get operation type for handler routing
	 */
	private getOperationType;
	/**
	 * Setup transport event listeners
	 */
	private setupTransportListeners;
	/**
	 * Handle incoming transport message
	 */
	private handleTransportMessage;
	/**
	 * Type guard for streaming operations
	 */
	private isStreamingOperation;
	/**
	 * Clean up resources
	 */
	destroy(): void;
}
/**
 * Timeline streaming handler for status updates
 */
export declare class TimelineStreamingHandler implements StreamingOperationHandler {
	private onStatusUpdate;
	private onStatusDelete;
	private onStatusEdit;
	type: string;
	constructor(
		onStatusUpdate: (status: UnifiedStatus) => void,
		onStatusDelete: (statusId: string) => void,
		onStatusEdit: (status: UnifiedStatus) => void
	);
	handle(operation: StreamingOperation): Promise<void>;
}
/**
 * Notification streaming handler
 */
export declare class NotificationStreamingHandler implements StreamingOperationHandler {
	private onNotificationUpdate;
	private onNotificationDelete;
	type: string;
	constructor(
		onNotificationUpdate: (notification: UnifiedNotification) => void,
		onNotificationDelete: (notificationId: string) => void
	);
	handle(operation: StreamingOperation): Promise<void>;
}
/**
 * Account streaming handler for user updates
 */
export declare class AccountStreamingHandler implements StreamingOperationHandler {
	private onAccountUpdate;
	private onAccountDelete;
	type: string;
	constructor(
		onAccountUpdate: (account: UnifiedAccount) => void,
		onAccountDelete: (accountId: string) => void
	);
	handle(operation: StreamingOperation): Promise<void>;
}
/**
 * Delete operation handler
 */
export declare class DeleteStreamingHandler implements StreamingOperationHandler {
	private onDelete;
	type: string;
	constructor(onDelete: (deleteOperation: StreamingDelete) => void);
	handle(operation: StreamingOperation): Promise<void>;
}
/**
 * Edit operation handler
 */
export declare class EditStreamingHandler implements StreamingOperationHandler {
	private onEdit;
	type: string;
	constructor(onEdit: (editOperation: StreamingEdit) => void);
	handle(operation: StreamingOperation): Promise<void>;
}
/**
 * State consistency manager for streaming operations
 */
export declare class StreamingStateManager {
	private statusCache;
	private accountCache;
	private notificationCache;
	/**
	 * Apply streaming operation with state consistency checks
	 */
	applyOperation(operation: StreamingOperation): Promise<{
		applied: boolean;
		conflicts: string[];
	}>;
	/**
	 * Apply delete operation
	 */
	private applyDelete;
	/**
	 * Apply edit operation with conflict detection
	 */
	private applyEdit;
	/**
	 * Apply update operation
	 */
	private applyUpdate;
	/**
	 * Get cached item by ID and type
	 */
	getCachedItem(
		id: string,
		type: 'status' | 'account' | 'notification'
	): UnifiedStatus | UnifiedAccount | UnifiedNotification | undefined;
	/**
	 * Clear all cached data
	 */
	clearCache(): void;
	/**
	 * Get cache statistics
	 */
	getCacheStats(): {
		statuses: number;
		accounts: number;
		notifications: number;
	};
}
//# sourceMappingURL=operations.d.ts.map
