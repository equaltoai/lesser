/**
 * Admin Streaming Store for Lesser real-time admin events
 * Handles metrics, cost alerts, moderation alerts, federation health,
 * performance monitoring, and infrastructure events
 */
import type { StreamingUpdate } from '../models/unified.js';
export interface MetricsUpdate {
	metricId: string;
	serviceName: string;
	metricType: string;
	subscriptionCategory: string;
	aggregationLevel: string;
	timestamp: string;
	count: number;
	sum: number;
	min: number;
	max: number;
	average: number;
	p50?: number;
	p95?: number;
	p99?: number;
	unit?: string;
	userCostMicrocents?: number;
	totalCostMicrocents?: number;
	userId?: string;
	tenantId?: string;
	instanceDomain?: string;
	dimensions: Array<{
		key: string;
		value: string;
	}>;
}
export interface ModerationAlert {
	id: string;
	severity: 'INFO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
	matchedText: string;
	confidence: number;
	suggestedAction: 'NONE' | 'FLAG' | 'HIDE' | 'REMOVE' | 'SHADOW_BAN' | 'REVIEW';
	timestamp: string;
	handled: boolean;
	pattern?: {
		id: string;
		pattern: string;
		type: string;
		severity: string;
	};
	content: unknown;
}
export interface CostAlert {
	id: string;
	type: string;
	amount: number;
	threshold: number;
	domain?: string;
	message: string;
	timestamp: string;
}
export interface BudgetAlert {
	id: string;
	domain: string;
	budgetUSD: number;
	spentUSD: number;
	percentUsed: number;
	projectedOverspend?: number;
	alertLevel: 'INFO' | 'WARNING' | 'CRITICAL';
	timestamp: string;
}
export interface FederationHealthUpdate {
	domain: string;
	previousStatus: 'HEALTHY' | 'WARNING' | 'CRITICAL' | 'OFFLINE' | 'UNKNOWN';
	currentStatus: 'HEALTHY' | 'WARNING' | 'CRITICAL' | 'OFFLINE' | 'UNKNOWN';
	timestamp: string;
	issues: Array<{
		type: string;
		severity: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
		description: string;
		detectedAt: string;
		impact: string;
	}>;
}
export interface ModerationQueueItem {
	id: string;
	reportCount: number;
	severity: 'INFO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
	priority: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
	deadline: string;
	content: unknown;
	assignedTo?: unknown;
}
export interface ThreatIntelligence {
	id: string;
	type: string;
	severity: 'INFO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
	source: string;
	description: string;
	affectedInstances: string[];
	mitigationSteps: string[];
	timestamp: string;
}
export interface PerformanceAlert {
	id: string;
	service:
		| 'GRAPHQL_API'
		| 'FEDERATION_DELIVERY'
		| 'MEDIA_PROCESSOR'
		| 'MODERATION_ENGINE'
		| 'SEARCH_INDEXER'
		| 'STREAMING_SERVICE';
	metric: string;
	threshold: number;
	actualValue: number;
	severity: 'INFO' | 'WARNING' | 'ERROR' | 'CRITICAL';
	timestamp: string;
}
export interface InfrastructureEvent {
	id: string;
	type: 'DEPLOYMENT' | 'SCALING' | 'FAILURE' | 'RECOVERY' | 'MAINTENANCE';
	service: string;
	description: string;
	impact: string;
	timestamp: string;
}
export interface AdminStreamingStoreState {
	recentMetrics: MetricsUpdate[];
	metricsByService: Map<string, MetricsUpdate[]>;
	metricsByCategory: Map<string, MetricsUpdate[]>;
	moderationAlerts: ModerationAlert[];
	unhandledModerationAlerts: ModerationAlert[];
	costAlerts: CostAlert[];
	budgetAlerts: BudgetAlert[];
	performanceAlerts: PerformanceAlert[];
	federationHealthUpdates: FederationHealthUpdate[];
	unhealthyInstances: Set<string>;
	moderationQueue: ModerationQueueItem[];
	moderationQueueByPriority: Map<string, ModerationQueueItem[]>;
	threats: ThreatIntelligence[];
	activeThreats: ThreatIntelligence[];
	infrastructureEvents: InfrastructureEvent[];
	recentFailures: InfrastructureEvent[];
}
export interface AdminStreamingStoreConfig {
	/** Maximum number of items to keep in memory per category */
	maxHistorySize?: number;
	/** Auto-acknowledge handled moderation alerts after this duration (ms) */
	autoAcknowledgeAfter?: number;
	/** Enable automatic alert deduplication */
	enableDeduplication?: boolean;
	/** Alert severity threshold for notifications */
	alertSeverityThreshold?: 'INFO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
}
type AdminEventMap = {
	metricsUpdate: MetricsUpdate;
	moderationAlert: ModerationAlert;
	costAlert: CostAlert;
	budgetAlert: BudgetAlert;
	federationHealthUpdate: FederationHealthUpdate;
	moderationQueueUpdate: ModerationQueueItem;
	threatIntelligence: ThreatIntelligence;
	performanceAlert: PerformanceAlert;
	infrastructureEvent: InfrastructureEvent;
	moderationAlertAcknowledged: {
		alertId: string;
	};
	cleared: undefined;
};
type AdminEventName = keyof AdminEventMap;
/**
 * Admin Streaming Store for managing real-time admin events
 */
export declare class AdminStreamingStore {
	private state;
	private config;
	private listeners;
	constructor(config?: AdminStreamingStoreConfig);
	/**
	 * Process incoming streaming update
	 */
	processStreamingUpdate(update: StreamingUpdate): void;
	/**
	 * Subscribe to specific admin event types
	 */
	on<TEvent extends AdminEventName>(
		event: TEvent,
		callback: (data: AdminEventMap[TEvent]) => void
	): () => void;
	/**
	 * Emit event to all listeners
	 */
	private emit;
	/**
	 * Handle metrics update
	 */
	private handleMetricsUpdate;
	/**
	 * Handle moderation alert
	 */
	private handleModerationAlert;
	/**
	 * Handle cost alert
	 */
	private handleCostAlert;
	/**
	 * Handle budget alert
	 */
	private handleBudgetAlert;
	/**
	 * Handle federation health update
	 */
	private handleFederationHealthUpdate;
	/**
	 * Handle moderation queue update
	 */
	private handleModerationQueueUpdate;
	/**
	 * Handle threat intelligence
	 */
	private handleThreatIntelligence;
	/**
	 * Handle performance alert
	 */
	private handlePerformanceAlert;
	/**
	 * Handle infrastructure event
	 */
	private handleInfrastructureEvent;
	/**
	 * Check if event should be filtered based on severity threshold
	 */
	private shouldFilterBySeverity;
	/**
	 * Get current store state (read-only)
	 */
	getState(): Readonly<AdminStreamingStoreState>;
	/**
	 * Get unhandled moderation alerts
	 */
	getUnhandledModerationAlerts(): ModerationAlert[];
	/**
	 * Get moderation queue items by priority
	 */
	getModerationQueueByPriority(
		priority: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL'
	): ModerationQueueItem[];
	/**
	 * Get metrics by service
	 */
	getMetricsByService(serviceName: string): MetricsUpdate[];
	/**
	 * Get metrics by category
	 */
	getMetricsByCategory(category: string): MetricsUpdate[];
	/**
	 * Get unhealthy instances
	 */
	getUnhealthyInstances(): string[];
	/**
	 * Get active threats
	 */
	getActiveThreats(): ThreatIntelligence[];
	/**
	 * Get recent infrastructure failures
	 */
	getRecentFailures(): InfrastructureEvent[];
	/**
	 * Acknowledge moderation alert
	 */
	acknowledgeModerationAlert(alertId: string): void;
	/**
	 * Clear all alerts and events
	 */
	clear(): void;
	/**
	 * Destroy the store and clean up resources
	 */
	destroy(): void;
}
/**
 * Create an admin streaming store instance
 */
export declare function createAdminStreamingStore(
	config?: AdminStreamingStoreConfig
): AdminStreamingStore;
export {};
//# sourceMappingURL=adminStreamingStore.d.ts.map
