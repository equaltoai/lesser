/**
 * Admin Streaming Store for Lesser real-time admin events
 * Handles metrics, cost alerts, moderation alerts, federation health,
 * performance monitoring, and infrastructure events
 */

import type { StreamingUpdate } from '../models/unified.js';

// Admin event types
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
	dimensions: Array<{ key: string; value: string }>;
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
	// Metrics
	recentMetrics: MetricsUpdate[];
	metricsByService: Map<string, MetricsUpdate[]>;
	metricsByCategory: Map<string, MetricsUpdate[]>;

	// Alerts
	moderationAlerts: ModerationAlert[];
	unhandledModerationAlerts: ModerationAlert[];
	costAlerts: CostAlert[];
	budgetAlerts: BudgetAlert[];
	performanceAlerts: PerformanceAlert[];

	// Federation & Health
	federationHealthUpdates: FederationHealthUpdate[];
	unhealthyInstances: Set<string>;

	// Moderation Queue
	moderationQueue: ModerationQueueItem[];
	moderationQueueByPriority: Map<string, ModerationQueueItem[]>;

	// Threat Intelligence
	threats: ThreatIntelligence[];
	activeThreats: ThreatIntelligence[];

	// Infrastructure
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
	moderationAlertAcknowledged: { alertId: string };
	cleared: undefined;
};

type AdminEventName = keyof AdminEventMap;

/**
 * Admin Streaming Store for managing real-time admin events
 */
export class AdminStreamingStore {
	private state: AdminStreamingStoreState = {
		recentMetrics: [],
		metricsByService: new Map(),
		metricsByCategory: new Map(),
		moderationAlerts: [],
		unhandledModerationAlerts: [],
		costAlerts: [],
		budgetAlerts: [],
		performanceAlerts: [],
		federationHealthUpdates: [],
		unhealthyInstances: new Set(),
		moderationQueue: [],
		moderationQueueByPriority: new Map(),
		threats: [],
		activeThreats: [],
		infrastructureEvents: [],
		recentFailures: [],
	};

	private config: Required<AdminStreamingStoreConfig>;
	private listeners = new Map<AdminEventName, Set<(data: unknown) => void>>();

	constructor(config: AdminStreamingStoreConfig = {}) {
		this.config = {
			maxHistorySize: config.maxHistorySize || 100,
			autoAcknowledgeAfter: config.autoAcknowledgeAfter || 3600000, // 1 hour
			enableDeduplication: config.enableDeduplication !== false,
			alertSeverityThreshold: config.alertSeverityThreshold || 'MEDIUM',
		};
	}

	/**
	 * Process incoming streaming update
	 */
	processStreamingUpdate(update: StreamingUpdate): void {
		switch (update.type) {
			case 'metricsUpdates':
				this.handleMetricsUpdate(update.payload as MetricsUpdate);
				break;
			case 'moderationAlerts':
				this.handleModerationAlert(update.payload as ModerationAlert);
				break;
			case 'costAlerts':
				this.handleCostAlert(update.payload as CostAlert);
				break;
			case 'budgetAlerts':
				this.handleBudgetAlert(update.payload as BudgetAlert);
				break;
			case 'federationHealthUpdates':
				this.handleFederationHealthUpdate(update.payload as FederationHealthUpdate);
				break;
			case 'moderationQueueUpdate':
				this.handleModerationQueueUpdate(update.payload as ModerationQueueItem);
				break;
			case 'threatIntelligence':
				this.handleThreatIntelligence(update.payload as ThreatIntelligence);
				break;
			case 'performanceAlert':
				this.handlePerformanceAlert(update.payload as PerformanceAlert);
				break;
			case 'infrastructureEvent':
				this.handleInfrastructureEvent(update.payload as InfrastructureEvent);
				break;
		}
	}

	/**
	 * Subscribe to specific admin event types
	 */
	on<TEvent extends AdminEventName>(
		event: TEvent,
		callback: (data: AdminEventMap[TEvent]) => void
	): () => void {
		let callbacks = this.listeners.get(event);
		if (!callbacks) {
			callbacks = new Set();
			this.listeners.set(event, callbacks);
		}

		const wrapped = (data: unknown): void => {
			callback(data as AdminEventMap[TEvent]);
		};

		callbacks.add(wrapped);

		return () => {
			const existing = this.listeners.get(event);
			existing?.delete(wrapped);
			if (existing && existing.size === 0) {
				this.listeners.delete(event);
			}
		};
	}

	/**
	 * Emit event to all listeners
	 */
	private emit<TEvent extends AdminEventName>(event: TEvent, data: AdminEventMap[TEvent]): void {
		this.listeners.get(event)?.forEach((callback) => {
			try {
				callback(data);
			} catch (error) {
				console.error(`Error in admin streaming event handler for ${event}:`, error);
			}
		});
	}

	/**
	 * Handle metrics update
	 */
	private handleMetricsUpdate(metric: MetricsUpdate): void {
		// Add to recent metrics
		this.state.recentMetrics.unshift(metric);
		if (this.state.recentMetrics.length > this.config.maxHistorySize) {
			this.state.recentMetrics = this.state.recentMetrics.slice(0, this.config.maxHistorySize);
		}

		// Index by service
		let serviceMetrics = this.state.metricsByService.get(metric.serviceName);
		if (!serviceMetrics) {
			serviceMetrics = [];
			this.state.metricsByService.set(metric.serviceName, serviceMetrics);
		}
		serviceMetrics.unshift(metric);
		if (serviceMetrics.length > this.config.maxHistorySize) {
			serviceMetrics.splice(this.config.maxHistorySize);
		}

		// Index by category
		let categoryMetrics = this.state.metricsByCategory.get(metric.subscriptionCategory);
		if (!categoryMetrics) {
			categoryMetrics = [];
			this.state.metricsByCategory.set(metric.subscriptionCategory, categoryMetrics);
		}
		categoryMetrics.unshift(metric);
		if (categoryMetrics.length > this.config.maxHistorySize) {
			categoryMetrics.splice(this.config.maxHistorySize);
		}

		this.emit('metricsUpdate', metric);
	}

	/**
	 * Handle moderation alert
	 */
	private handleModerationAlert(alert: ModerationAlert): void {
		if (this.shouldFilterBySeverity(alert.severity)) {
			return;
		}

		// Add to alerts
		this.state.moderationAlerts.unshift(alert);
		if (this.state.moderationAlerts.length > this.config.maxHistorySize) {
			this.state.moderationAlerts = this.state.moderationAlerts.slice(
				0,
				this.config.maxHistorySize
			);
		}

		// Track unhandled alerts
		if (!alert.handled) {
			this.state.unhandledModerationAlerts.unshift(alert);
		}

		this.emit('moderationAlert', alert);
	}

	/**
	 * Handle cost alert
	 */
	private handleCostAlert(alert: CostAlert): void {
		this.state.costAlerts.unshift(alert);
		if (this.state.costAlerts.length > this.config.maxHistorySize) {
			this.state.costAlerts = this.state.costAlerts.slice(0, this.config.maxHistorySize);
		}

		this.emit('costAlert', alert);
	}

	/**
	 * Handle budget alert
	 */
	private handleBudgetAlert(alert: BudgetAlert): void {
		if (this.shouldFilterBySeverity(alert.alertLevel)) {
			return;
		}

		this.state.budgetAlerts.unshift(alert);
		if (this.state.budgetAlerts.length > this.config.maxHistorySize) {
			this.state.budgetAlerts = this.state.budgetAlerts.slice(0, this.config.maxHistorySize);
		}

		this.emit('budgetAlert', alert);
	}

	/**
	 * Handle federation health update
	 */
	private handleFederationHealthUpdate(update: FederationHealthUpdate): void {
		this.state.federationHealthUpdates.unshift(update);
		if (this.state.federationHealthUpdates.length > this.config.maxHistorySize) {
			this.state.federationHealthUpdates = this.state.federationHealthUpdates.slice(
				0,
				this.config.maxHistorySize
			);
		}

		// Track unhealthy instances
		if (update.currentStatus !== 'HEALTHY') {
			this.state.unhealthyInstances.add(update.domain);
		} else {
			this.state.unhealthyInstances.delete(update.domain);
		}

		this.emit('federationHealthUpdate', update);
	}

	/**
	 * Handle moderation queue update
	 */
	private handleModerationQueueUpdate(item: ModerationQueueItem): void {
		// Update or add to queue
		const existingIndex = this.state.moderationQueue.findIndex((i) => i.id === item.id);
		if (existingIndex >= 0) {
			this.state.moderationQueue[existingIndex] = item;
		} else {
			this.state.moderationQueue.push(item);
		}

		// Sort by priority and deadline
		this.state.moderationQueue.sort((a, b) => {
			const priorityOrder = { CRITICAL: 0, HIGH: 1, MEDIUM: 2, LOW: 3 };
			const aPriority = priorityOrder[a.priority];
			const bPriority = priorityOrder[b.priority];
			if (aPriority !== bPriority) {
				return aPriority - bPriority;
			}
			return new Date(a.deadline).getTime() - new Date(b.deadline).getTime();
		});

		// Index by priority
		this.state.moderationQueueByPriority.clear();
		for (const queueItem of this.state.moderationQueue) {
			let priorityQueue = this.state.moderationQueueByPriority.get(queueItem.priority);
			if (!priorityQueue) {
				priorityQueue = [];
				this.state.moderationQueueByPriority.set(queueItem.priority, priorityQueue);
			}
			priorityQueue.push(queueItem);
		}

		this.emit('moderationQueueUpdate', item);
	}

	/**
	 * Handle threat intelligence
	 */
	private handleThreatIntelligence(threat: ThreatIntelligence): void {
		if (this.shouldFilterBySeverity(threat.severity)) {
			return;
		}

		this.state.threats.unshift(threat);
		if (this.state.threats.length > this.config.maxHistorySize) {
			this.state.threats = this.state.threats.slice(0, this.config.maxHistorySize);
		}

		// Track active threats (high severity)
		if (threat.severity === 'HIGH' || threat.severity === 'CRITICAL') {
			this.state.activeThreats.unshift(threat);
			if (this.state.activeThreats.length > 20) {
				this.state.activeThreats = this.state.activeThreats.slice(0, 20);
			}
		}

		this.emit('threatIntelligence', threat);
	}

	/**
	 * Handle performance alert
	 */
	private handlePerformanceAlert(alert: PerformanceAlert): void {
		if (this.shouldFilterBySeverity(alert.severity)) {
			return;
		}

		this.state.performanceAlerts.unshift(alert);
		if (this.state.performanceAlerts.length > this.config.maxHistorySize) {
			this.state.performanceAlerts = this.state.performanceAlerts.slice(
				0,
				this.config.maxHistorySize
			);
		}

		this.emit('performanceAlert', alert);
	}

	/**
	 * Handle infrastructure event
	 */
	private handleInfrastructureEvent(event: InfrastructureEvent): void {
		this.state.infrastructureEvents.unshift(event);
		if (this.state.infrastructureEvents.length > this.config.maxHistorySize) {
			this.state.infrastructureEvents = this.state.infrastructureEvents.slice(
				0,
				this.config.maxHistorySize
			);
		}

		// Track recent failures
		if (event.type === 'FAILURE') {
			this.state.recentFailures.unshift(event);
			if (this.state.recentFailures.length > 10) {
				this.state.recentFailures = this.state.recentFailures.slice(0, 10);
			}
		}

		this.emit('infrastructureEvent', event);
	}

	/**
	 * Check if event should be filtered based on severity threshold
	 */
	private shouldFilterBySeverity(severity: string): boolean {
		const severityOrder = { INFO: 0, LOW: 1, MEDIUM: 2, HIGH: 3, CRITICAL: 4 };
		const threshold = severityOrder[this.config.alertSeverityThreshold];
		const eventSeverity = severityOrder[severity as keyof typeof severityOrder];
		return eventSeverity < threshold;
	}

	/**
	 * Get current store state (read-only)
	 */
	getState(): Readonly<AdminStreamingStoreState> {
		return {
			...this.state,
			metricsByService: new Map(this.state.metricsByService),
			metricsByCategory: new Map(this.state.metricsByCategory),
			unhealthyInstances: new Set(this.state.unhealthyInstances),
			moderationQueueByPriority: new Map(this.state.moderationQueueByPriority),
		};
	}

	/**
	 * Get unhandled moderation alerts
	 */
	getUnhandledModerationAlerts(): ModerationAlert[] {
		return [...this.state.unhandledModerationAlerts];
	}

	/**
	 * Get moderation queue items by priority
	 */
	getModerationQueueByPriority(
		priority: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL'
	): ModerationQueueItem[] {
		return this.state.moderationQueueByPriority.get(priority) || [];
	}

	/**
	 * Get metrics by service
	 */
	getMetricsByService(serviceName: string): MetricsUpdate[] {
		return this.state.metricsByService.get(serviceName) || [];
	}

	/**
	 * Get metrics by category
	 */
	getMetricsByCategory(category: string): MetricsUpdate[] {
		return this.state.metricsByCategory.get(category) || [];
	}

	/**
	 * Get unhealthy instances
	 */
	getUnhealthyInstances(): string[] {
		return Array.from(this.state.unhealthyInstances);
	}

	/**
	 * Get active threats
	 */
	getActiveThreats(): ThreatIntelligence[] {
		return [...this.state.activeThreats];
	}

	/**
	 * Get recent infrastructure failures
	 */
	getRecentFailures(): InfrastructureEvent[] {
		return [...this.state.recentFailures];
	}

	/**
	 * Acknowledge moderation alert
	 */
	acknowledgeModerationAlert(alertId: string): void {
		const alert = this.state.moderationAlerts.find((a) => a.id === alertId);
		if (alert) {
			alert.handled = true;
			this.state.unhandledModerationAlerts = this.state.unhandledModerationAlerts.filter(
				(a) => a.id !== alertId
			);
			this.emit('moderationAlertAcknowledged', { alertId });
		}
	}

	/**
	 * Clear all alerts and events
	 */
	clear(): void {
		this.state = {
			recentMetrics: [],
			metricsByService: new Map(),
			metricsByCategory: new Map(),
			moderationAlerts: [],
			unhandledModerationAlerts: [],
			costAlerts: [],
			budgetAlerts: [],
			performanceAlerts: [],
			federationHealthUpdates: [],
			unhealthyInstances: new Set(),
			moderationQueue: [],
			moderationQueueByPriority: new Map(),
			threats: [],
			activeThreats: [],
			infrastructureEvents: [],
			recentFailures: [],
		};
		this.emit('cleared', undefined);
	}

	/**
	 * Destroy the store and clean up resources
	 */
	destroy(): void {
		this.clear();
		this.listeners.clear();
	}
}

/**
 * Create an admin streaming store instance
 */
export function createAdminStreamingStore(config?: AdminStreamingStoreConfig): AdminStreamingStore {
	return new AdminStreamingStore(config);
}
