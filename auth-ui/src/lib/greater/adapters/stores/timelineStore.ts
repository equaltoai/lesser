/**
 * Timeline Store - Reactive state management for timeline data with streaming updates
 * Built for Svelte 5 runes compatibility with fallback support
 */

import type {
	TimelineStore,
	TimelineState,
	TimelineItem,
	TimelineConfig,
	StreamingEdit,
	JsonPatch,
	LesserTimelineMetadata,
	TimelineSource,
	TimelinePageInfo,
} from './types';
import { unifiedStatusToTimelineItem } from './unifiedToTimeline.js';
import { mapLesserObject } from '../mappers/lesser/mappers.js';
import { convertGraphQLObjectToLesser } from '../mappers/lesser/graphqlConverters.js';
import { mapLesserStreamingUpdate } from '../mappers/lesser/mappers.js';
import type { LesserStreamingUpdate } from '../mappers/lesser/types.js';
import type { UnifiedStatus } from '../models/unified.js';
import type { WebSocketEvent } from '../types.js';
import type {
	TimelineUpdatesSubscription,
	QuoteActivitySubscription,
	HashtagActivitySubscription,
	ListUpdatesSubscription,
	RelationshipUpdatesSubscription,
} from '../graphql/generated/types.js';
import { fetchTimelinePage } from './fetchHelpers.js';

type TimelineUpdatePayload = TimelineUpdatesSubscription['timelineUpdates'];
type QuoteActivityPayload = QuoteActivitySubscription['quoteActivity'];
type HashtagActivityPayload = HashtagActivitySubscription['hashtagActivity'];
type ListUpdatePayload = ListUpdatesSubscription['listUpdates'];
type RelationshipUpdatePayload = RelationshipUpdatesSubscription['relationshipUpdates'];
type TimelineObjectLike =
	| TimelineUpdatePayload
	| NonNullable<QuoteActivityPayload['quote']>
	| HashtagActivityPayload['post'];
type MetadataEnhancer = (metadata: LesserTimelineMetadata) => LesserTimelineMetadata;

// Simple reactive state implementation that works everywhere
class ReactiveState<T> {
	private _value: T;
	private _subscribers = new Set<(value: T) => void>();

	constructor(initialValue: T) {
		this._value = initialValue;
	}

	get value(): T {
		return this._value;
	}

	set value(newValue: T) {
		this._value = newValue;
		this._subscribers.forEach((callback) => {
			try {
				callback(newValue);
			} catch (error) {
				console.error('Error in reactive state subscriber:', error);
			}
		});
	}

	subscribe(callback: (value: T) => void): () => void {
		this._subscribers.add(callback);
		// Call immediately with current value
		callback(this._value);

		return () => {
			this._subscribers.delete(callback);
		};
	}

	update(updater: (current: T) => T): void {
		this.value = updater(this._value);
	}
}

export function createTimelineStore(config: TimelineConfig): TimelineStore {
	// Create reactive state
	const state = new ReactiveState<TimelineState>({
		items: config.initialItems || [],
		visibleItems: [],
		totalCount: 0,
		isLoading: false,
		error: null,
		isStreaming: false,
		lastSync: null,
		pageInfo: {
			endCursor: config.initialPageInfo?.endCursor ?? null,
			hasNextPage: config.initialPageInfo?.hasNextPage ?? true,
		},
		virtualWindow: {
			startIndex: 0,
			endIndex: 0,
			itemHeight: config.itemHeight || 50,
			containerHeight: config.containerHeight || 400,
		},
	});

	// Initialize computed values
	updateDerivedValues();

	// Transport event handlers
	let streamingUnsubscribers: (() => void)[] = [];
	let updateDebounceTimer: ReturnType<typeof setTimeout> | null = null;
	let pendingUpdates: StreamingEdit[] = [];
	const timelineSource: TimelineSource | undefined = config.timeline;
	const pageSize = config.pageSize ?? 20;
	const deletionMode: NonNullable<TimelineConfig['deletionMode']> =
		config.deletionMode ?? (config.adapter ? 'tombstone' : 'remove');
	const sourceMetadataEnhancer = createSourceMetadataEnhancer(timelineSource);

	const isRecord = (value: unknown): value is Record<string, unknown> =>
		typeof value === 'object' && value !== null;

	const convertTimelineObject = (object: TimelineObjectLike) =>
		convertGraphQLObjectToLesser(object);

	function mapTimelineObject(object: TimelineObjectLike | null | undefined): UnifiedStatus | null {
		if (!object) return null;

		const lesserObject = convertTimelineObject(object);
		if (!lesserObject) {
			return null;
		}

		const result = mapLesserObject(lesserObject);
		if (!result.success || !result.data) {
			if (result.error) {
				console.warn('[TimelineStore] Failed to map Lesser timeline object', result.error);
			}
			return null;
		}

		return result.data;
	}

	function resolveTimestamp(value?: string | number | null): number {
		if (typeof value === 'number' && Number.isFinite(value)) {
			return value;
		}

		if (typeof value === 'string') {
			const parsed = Date.parse(value);
			if (!Number.isNaN(parsed)) {
				return parsed;
			}
		}

		return Date.now();
	}

	function createSourceMetadataEnhancer(source?: TimelineSource): MetadataEnhancer | undefined {
		if (!source) return undefined;

		return (metadata: LesserTimelineMetadata) => {
			const next: LesserTimelineMetadata = { ...(metadata ?? {}) };
			let changed = false;

			if (source.type === 'list' && source.id) {
				const lists = new Set([...(next.listMemberships ?? []), source.id]);
				next.listMemberships = Array.from(lists);
				changed = true;
			}

			if (source.type === 'hashtag') {
				const tag = source.hashtag ?? source.id;
				if (tag) {
					const tags = new Set([...(next.hashtags ?? []), tag]);
					next.hashtags = Array.from(tags);
					changed = true;
				}
			}

			return changed ? next : metadata;
		};
	}

	function normalizePageInfo(info?: Partial<TimelinePageInfo> | null): TimelinePageInfo {
		return {
			endCursor: info?.endCursor ?? null,
			hasNextPage: info?.hasNextPage ?? false,
		};
	}

	function runMetadataEnhancer(
		existing: LesserTimelineMetadata | undefined,
		enhancer?: MetadataEnhancer
	): LesserTimelineMetadata | undefined {
		if (!enhancer) {
			return existing;
		}

		const current: LesserTimelineMetadata = { ...(existing ?? {}) };
		const enhanced = enhancer(current);

		return Object.keys(enhanced).length > 0 ? enhanced : undefined;
	}

	function upsertUnifiedStatus(
		unifiedStatus: UnifiedStatus,
		metadataEnhancer?: MetadataEnhancer
	): void {
		const baseItem = unifiedStatusToTimelineItem(unifiedStatus);
		const existingItem = state.value.items.find((item) => item.id === unifiedStatus.id);
		const existingMetadata = existingItem?.metadata;
		const baseMetadata = baseItem.metadata;

		const mergedLesser =
			existingMetadata?.lesser || baseMetadata?.lesser
				? {
						...(existingMetadata?.lesser ?? {}),
						...(baseMetadata?.lesser ?? {}),
					}
				: undefined;

		const finalLesser = runMetadataEnhancer(mergedLesser, metadataEnhancer);

		const combinedMetadata = (() => {
			if (!existingMetadata && !baseMetadata && !finalLesser) {
				return undefined;
			}

			const merged: TimelineItem['metadata'] = {
				...(existingMetadata ?? {}),
				...(baseMetadata ?? {}),
			};

			if (finalLesser) {
				merged.lesser = finalLesser;
			} else {
				delete merged.lesser;
			}

			return Object.keys(merged).length > 0 ? merged : undefined;
		})();

		const edit: StreamingEdit = {
			type: existingItem ? 'replace' : 'add',
			itemId: unifiedStatus.id,
			data: {
				...baseItem,
				metadata: combinedMetadata,
			},
			timestamp: resolveTimestamp(unifiedStatus.createdAt),
			userId: unifiedStatus.account?.id,
		};

		applyStreamingEdit(edit);
	}

	function removeItemsByPredicate(predicate: (item: TimelineItem) => boolean): void {
		let mutated = false;

		state.update((current) => {
			const filtered = current.items.filter((item) => {
				const shouldRemove = predicate(item);
				if (shouldRemove) mutated = true;
				return !shouldRemove;
			});

			return mutated
				? {
						...current,
						items: filtered,
					}
				: current;
		});

		if (mutated) {
			updateDerivedValues();
		}
	}

	function updateItemsMetadata(
		predicate: (item: TimelineItem) => boolean,
		updater: (metadata: LesserTimelineMetadata | undefined) => LesserTimelineMetadata | undefined
	): void {
		let mutated = false;

		state.update((current) => {
			const items = current.items.map((item) => {
				if (!predicate(item)) {
					return item;
				}

				const currentMetadata = item.metadata ?? undefined;
				const currentLesser = currentMetadata?.lesser;

				const nextLesser = updater(currentLesser);
				if (nextLesser === currentLesser) {
					return item;
				}

				mutated = true;

				const metadata: TimelineItem['metadata'] | undefined = (() => {
					if (!currentMetadata && !nextLesser) return undefined;

					const merged: TimelineItem['metadata'] = {
						...(currentMetadata ?? {}),
					};

					if (nextLesser) {
						merged.lesser = nextLesser;
					} else {
						delete merged.lesser;
					}

					return Object.keys(merged).length > 0 ? merged : undefined;
				})();

				return {
					...item,
					metadata,
				};
			});

			return mutated ? { ...current, items } : current;
		});

		if (mutated) {
			updateDerivedValues();
		}
	}

	function isTombstone(item?: TimelineItem | null): boolean {
		if (!item) return false;
		return item.type === 'tombstone' || item.metadata?.lesser?.isDeleted === true;
	}

	function buildTombstoneEdit(
		itemId: string,
		options?: { deletedAt?: string; formerType?: string }
	): StreamingEdit {
		const existingItem = state.value.items.find((candidate) => candidate.id === itemId);
		if (isTombstone(existingItem) && existingItem) {
			return {
				type: 'replace',
				itemId,
				data: existingItem,
				timestamp: existingItem.timestamp,
			};
		}

		const deletedAt = options?.deletedAt ?? new Date().toISOString();
		const formerType = options?.formerType ?? existingItem?.type ?? 'status';
		const baseMetadata: TimelineItem['metadata'] = { ...(existingItem?.metadata ?? {}) };
		const metadata: TimelineItem['metadata'] = {
			...baseMetadata,
			lesser: {
				...(baseMetadata?.lesser ?? {}),
				isDeleted: true,
				deletedAt,
				formerType,
			},
		};

		const content =
			existingItem?.content && typeof existingItem.content === 'object'
				? {
						...(existingItem.content as Record<string, unknown>),
						id: itemId,
						deletedAt,
						formerType,
					}
				: { id: itemId, deletedAt, formerType };

		return {
			type: 'replace',
			itemId,
			data: {
				id: itemId,
				type: 'tombstone',
				content,
				metadata,
				timestamp: existingItem?.timestamp ?? Date.now(),
			},
			timestamp: existingItem?.timestamp ?? Date.now(),
		};
	}

	function getStatusFromItem(item: TimelineItem): UnifiedStatus | null {
		if (!item?.content || typeof item.content !== 'object') {
			return null;
		}

		const maybeStatus = item.content as Partial<UnifiedStatus>;
		return typeof maybeStatus.id === 'string' ? (maybeStatus as UnifiedStatus) : null;
	}

	function updateDerivedValues(): void {
		const currentState = state.value;

		// Sort items by timestamp (newest first)
		const sortedItems = [...currentState.items].sort((a, b) => b.timestamp - a.timestamp);

		// Calculate visible range with overscan
		const { startIndex, endIndex } = currentState.virtualWindow;
		const overscan = config.overscan || 5;
		const start = Math.max(0, startIndex - overscan);
		const end = Math.min(sortedItems.length, endIndex + overscan);
		const visibleItems = sortedItems.slice(start, end);

		// Update state with computed values
		state.update((current) => ({
			...current,
			items: sortedItems,
			visibleItems,
			totalCount: sortedItems.length,
		}));
	}

	// Debounced update processor
	function processPendingUpdates(): void {
		if (pendingUpdates.length === 0) return;

		const updates = [...pendingUpdates];
		pendingUpdates = [];

		// Group updates by type and item
		const groupedUpdates = new Map<string, StreamingEdit[]>();

		for (const update of updates) {
			const key = `${update.type}-${update.itemId}`;
			let group = groupedUpdates.get(key);
			if (!group) {
				group = [];
				groupedUpdates.set(key, group);
			}
			group.push(update);
		}

		// Apply grouped updates
		for (const [, edits] of groupedUpdates) {
			const latestEdit = edits[edits.length - 1]; // Use most recent edit
			if (latestEdit) {
				applyStreamingEditInternal(latestEdit);
			}
		}

		updateDerivedValues();
	}

	function scheduleUpdate(edit: StreamingEdit): void {
		pendingUpdates.push(edit);

		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
		}

		updateDebounceTimer = setTimeout(() => {
			processPendingUpdates();
			updateDebounceTimer = null;
		}, config.updateDebounceMs || 100);
	}

	function applyStreamingEditInternal(edit: StreamingEdit): void {
		state.update((current) => {
			const itemIndex = current.items.findIndex((item) => item.id === edit.itemId);
			let newItems = [...current.items];

			const buildTimelineItem = (
				itemId: string,
				timestamp: number,
				itemType: string,
				content: unknown,
				metadata: TimelineItem['metadata'] | undefined,
				version: number | undefined,
				isOptimistic: boolean | undefined
			): TimelineItem => {
				const item: TimelineItem = {
					id: itemId,
					type: itemType,
					timestamp,
					content,
				};

				if (typeof metadata !== 'undefined') {
					item.metadata = metadata;
				}

				if (typeof version !== 'undefined') {
					item.version = version;
				}

				if (typeof isOptimistic === 'boolean') {
					item.isOptimistic = isOptimistic;
				}

				return item;
			};

			switch (edit.type) {
				case 'add': {
					if (itemIndex === -1 && isRecord(edit.data)) {
						if (!Object.prototype.hasOwnProperty.call(edit.data, 'content')) {
							break;
						}

						const typeValue = edit.data['type'];
						const contentValue = edit.data['content'];
						const optimisticValue = edit.data['isOptimistic'];

						const metadataProvided = Object.prototype.hasOwnProperty.call(edit.data, 'metadata');
						let metadata: TimelineItem['metadata'] | undefined;
						if (metadataProvided) {
							const rawMetadata = edit.data['metadata'];
							if (rawMetadata === null || rawMetadata === undefined) {
								metadata = undefined;
							} else if (isRecord(rawMetadata)) {
								metadata = rawMetadata as TimelineItem['metadata'];
							} else {
								break;
							}
						}

						const versionProvided = Object.prototype.hasOwnProperty.call(edit.data, 'version');
						let version: number | undefined;
						if (versionProvided) {
							const rawVersion = edit.data['version'];
							if (rawVersion === null || rawVersion === undefined) {
								version = undefined;
							} else if (typeof rawVersion === 'number') {
								version = rawVersion;
							} else {
								break;
							}
						}

						const typeString = typeof typeValue === 'string' ? typeValue : 'default';
						const isOptimistic = typeof optimisticValue === 'boolean' ? optimisticValue : false;
						const effectiveMetadata = metadataProvided ? metadata : undefined;
						const effectiveVersion = versionProvided ? version : undefined;

						const newItem = buildTimelineItem(
							edit.itemId,
							edit.timestamp,
							typeString,
							contentValue,
							effectiveMetadata,
							effectiveVersion,
							isOptimistic
						);

						newItems.push(newItem);
					}
					break;
				}

				case 'replace': {
					if (!isRecord(edit.data)) break;

					const typeValue = edit.data['type'];
					const contentProvided = Object.prototype.hasOwnProperty.call(edit.data, 'content');
					const metadataProvided = Object.prototype.hasOwnProperty.call(edit.data, 'metadata');
					const versionProvided = Object.prototype.hasOwnProperty.call(edit.data, 'version');
					const optimisticProvided = Object.prototype.hasOwnProperty.call(
						edit.data,
						'isOptimistic'
					);

					let metadata: TimelineItem['metadata'] | undefined;
					if (metadataProvided) {
						const rawMetadata = edit.data['metadata'];
						if (rawMetadata === null || rawMetadata === undefined) {
							metadata = undefined;
						} else if (isRecord(rawMetadata)) {
							metadata = rawMetadata as TimelineItem['metadata'];
						} else {
							break;
						}
					}

					let version: number | undefined;
					if (versionProvided) {
						const rawVersion = edit.data['version'];
						if (rawVersion === null || rawVersion === undefined) {
							version = undefined;
						} else if (typeof rawVersion === 'number') {
							version = rawVersion;
						} else {
							break;
						}
					}

					const currentItem = itemIndex !== -1 ? newItems[itemIndex] : undefined;
					const nextType =
						typeof typeValue === 'string' ? typeValue : (currentItem?.type ?? 'default');
					const nextContent = contentProvided
						? edit.data['content']
						: (currentItem?.content ?? edit.data['content']);
					const nextMetadata = metadataProvided ? metadata : currentItem?.metadata;
					const nextVersion = versionProvided ? version : currentItem?.version;
					const rawOptimistic = optimisticProvided
						? edit.data['isOptimistic']
						: currentItem?.isOptimistic;
					const nextIsOptimistic =
						optimisticProvided && typeof rawOptimistic !== 'boolean'
							? undefined
							: (rawOptimistic as boolean | undefined);

					const updatedItem = buildTimelineItem(
						edit.itemId,
						edit.timestamp,
						nextType,
						nextContent,
						nextMetadata,
						typeof nextVersion === 'number' ? nextVersion : undefined,
						nextIsOptimistic ?? currentItem?.isOptimistic
					);

					if (itemIndex !== -1) {
						newItems[itemIndex] = updatedItem;
					} else {
						newItems.push(updatedItem);
					}
					break;
				}

				case 'delete':
					if (itemIndex !== -1) {
						newItems = newItems.filter((item) => item.id !== edit.itemId);
					}
					break;

				case 'patch':
					if (edit.patches && itemIndex !== -1) {
						const item = newItems[itemIndex];
						if (item) {
							const patchedItem = applyPatches(item, edit.patches);
							newItems[itemIndex] = patchedItem;
						}
					}
					break;
			}

			return {
				...current,
				items: newItems,
			};
		});
	}

	function applyPatches(item: TimelineItem, patches: JsonPatch[]): TimelineItem {
		let result = { ...item } as TimelineItemMutable;

		for (const patch of patches) {
			result = applyJsonPatch(result, patch);
		}

		return result;
	}

	const isJsonObject = (value: unknown): value is Record<string, unknown> =>
		typeof value === 'object' && value !== null && !Array.isArray(value);

	type TimelineItemMutable = TimelineItem & Record<string, unknown>;

	function applyJsonPatch(obj: TimelineItemMutable, patch: JsonPatch): TimelineItemMutable {
		const path = patch.path.split('/').filter((p) => p !== '');

		switch (patch.op) {
			case 'replace':
			case 'add':
				return setNestedValue(obj, path, patch.value);

			case 'remove':
				return removeNestedValue(obj, path);

			default:
				console.warn(`Unsupported JSON patch operation: ${patch.op}`);
				return obj;
		}
	}

	function setNestedValue(
		obj: TimelineItemMutable,
		path: string[],
		value: unknown
	): TimelineItemMutable {
		if (path.length === 0) return value as TimelineItemMutable;

		const result = { ...obj } as TimelineItemMutable;
		let current: Record<string, unknown> = result;

		for (let i = 0; i < path.length - 1; i++) {
			const key = path[i];
			if (!key) {
				continue;
			}

			const existing = current[key];
			if (!isJsonObject(existing)) {
				if (key) {
					current[key] = {};
				}
			} else {
				current[key] = { ...existing };
			}
			if (key) {
				current = current[key] as Record<string, unknown>;
			}
		}

		const lastKey = path[path.length - 1];
		if (lastKey) {
			current[lastKey] = value;
		}
		return result;
	}

	function removeNestedValue(obj: TimelineItemMutable, path: string[]): TimelineItemMutable {
		if (path.length === 0) return obj;
		if (path.length === 1) {
			const result = { ...obj } as TimelineItemMutable;
			const key = path[0];
			if (key) {
				delete result[key];
			}
			return result;
		}

		const result = { ...obj } as TimelineItemMutable;
		let current: Record<string, unknown> = result;

		for (let i = 0; i < path.length - 1; i++) {
			const key = path[i];
			if (!key) {
				continue;
			}

			const existing = current[key];
			if (!isJsonObject(existing)) {
				return result;
			}

			current[key] = { ...existing };
			current = current[key] as Record<string, unknown>;
		}

		const lastKey = path[path.length - 1];
		if (lastKey) {
			delete current[lastKey];
		}

		return result;
	}

	// Store methods
	function addItem(itemData: Omit<TimelineItem, 'id' | 'timestamp'>): string {
		const id = generateId();
		const timestamp = Date.now();

		const newItem: TimelineItem = {
			...itemData,
			id,
			timestamp,
			isOptimistic: true,
		};

		state.update((current) => ({
			...current,
			items: [...current.items, newItem],
		}));

		updateDerivedValues();

		// Send to server if streaming is active
		if (state.value.isStreaming && config.transportManager) {
			const edit: StreamingEdit = {
				type: 'add',
				itemId: id,
				data: newItem,
				timestamp,
				userId: 'current-user', // This should come from auth context
			};

			try {
				config.transportManager.send({
					type: 'timeline_edit',
					data: edit,
				});
			} catch (error) {
				// Remove optimistic item on send failure
				state.update((current) => ({
					...current,
					items: current.items.filter((item) => item.id !== id),
					error: error as Error,
				}));
				updateDerivedValues();
				throw error;
			}
		}

		return id;
	}

	function replaceItem(id: string, updates: Partial<TimelineItem>): boolean {
		const currentState = state.value;
		const itemIndex = currentState.items.findIndex((item) => item.id === id);
		if (itemIndex === -1) return false;

		const timestamp = Date.now();
		const existingItem = currentState.items[itemIndex];
		if (!existingItem) return false;

		const updatedItem: TimelineItem = {
			...existingItem,
			...updates,
			id, // Preserve ID
			timestamp, // Update timestamp
			type: (updates.type || existingItem.type) as string,
		};

		const newItems = currentState.items.map((item, index) =>
			index === itemIndex ? updatedItem : item
		);

		state.update((current) => ({
			...current,
			items: newItems,
		}));

		updateDerivedValues();

		// Send to server if streaming is active
		if (currentState.isStreaming && config.transportManager) {
			const edit: StreamingEdit = {
				type: 'replace',
				itemId: id,
				data: updatedItem,
				timestamp,
				userId: 'current-user',
			};

			try {
				config.transportManager.send({
					type: 'timeline_edit',
					data: edit,
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				throw error;
			}
		}

		return true;
	}

	function deleteItem(id: string): boolean {
		const currentState = state.value;
		const itemExists = currentState.items.some((item) => item.id === id);
		if (!itemExists) return false;

		state.update((current) => ({
			...current,
			items: current.items.filter((item) => item.id !== id),
		}));

		updateDerivedValues();

		// Send to server if streaming is active
		if (currentState.isStreaming && config.transportManager) {
			const edit: StreamingEdit = {
				type: 'delete',
				itemId: id,
				timestamp: Date.now(),
				userId: 'current-user',
			};

			try {
				config.transportManager.send({
					type: 'timeline_edit',
					data: edit,
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				throw error;
			}
		}

		return true;
	}

	async function deleteStatus(id: string): Promise<void> {
		if (!config.adapter) {
			throw new Error('Timeline adapter is required to delete a status');
		}

		try {
			await config.adapter.deleteObject(id);
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
			}));
			throw error;
		}

		if (deletionMode === 'tombstone') {
			const tombstoneEdit = buildTombstoneEdit(id, { deletedAt: new Date().toISOString() });
			applyStreamingEdit(tombstoneEdit);
		} else {
			deleteItem(id);
		}
	}

	function normalizeEdit(edit: StreamingEdit): StreamingEdit | null {
		if (edit.type === 'delete') {
			if (deletionMode === 'remove') {
				return edit;
			}
			return buildTombstoneEdit(edit.itemId, {
				deletedAt: new Date(edit.timestamp).toISOString(),
				formerType: 'status',
			});
		}

		return edit;
	}

	function applyStreamingEdit(edit: StreamingEdit): void {
		const normalized = normalizeEdit(edit);
		if (!normalized) return;
		scheduleUpdate(normalized);
	}

	function updateVirtualWindow(startIndex: number, endIndex: number): void {
		state.update((current) => ({
			...current,
			virtualWindow: {
				...current.virtualWindow,
				startIndex,
				endIndex,
			},
		}));

		updateDerivedValues();
	}

	async function fetchPage(after: string | null, replaceExisting = false): Promise<void> {
		if (!config.adapter || !timelineSource) {
			const error = new Error('Timeline adapter and source are required to fetch pages');
			console.error('[timelineStore fetchPage] Missing adapter or source!', {
				hasAdapter: !!config.adapter,
				timelineSource,
			});
			state.update((current) => ({
				...current,
				isLoading: false,
				error,
			}));
			throw error;
		}

		const metadataEnhancer = sourceMetadataEnhancer;
		const result = await fetchTimelinePage({
			adapter: config.adapter,
			source: timelineSource,
			pageSize,
			after,
		});

		if (replaceExisting) {
			state.update((current) => ({
				...current,
				items: [],
			}));
		}

		for (const status of result.items) {
			upsertUnifiedStatus(status, metadataEnhancer);
		}

		state.update((current) => ({
			...current,
			isLoading: false,
			error: null,
			lastSync: Date.now(),
			pageInfo: normalizePageInfo(result.pageInfo),
		}));

		updateDerivedValues();
	}

	async function loadMore(): Promise<void> {
		if (state.value.isLoading) return;
		const { pageInfo } = state.value;
		if (!pageInfo.hasNextPage && pageInfo.endCursor) {
			return;
		}

		state.update((current) => ({
			...current,
			isLoading: true,
			error: null,
		}));

		try {
			await fetchPage(state.value.pageInfo.endCursor ?? null, false);
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isLoading: false,
			}));
			throw error;
		}
	}

	async function refresh(): Promise<void> {
		if (state.value.isLoading) {
			return;
		}

		state.update((current) => ({
			...current,
			isLoading: true,
			error: null,
			pageInfo: {
				...normalizePageInfo(state.value.pageInfo),
				endCursor: null,
			},
			items: [],
		}));

		try {
			await fetchPage(null, true);
		} catch (error) {
			console.error('[timelineStore refresh] fetchPage error:', error);
			state.update((current) => ({
				...current,
				error: error as Error,
				isLoading: false,
			}));
			throw error;
		}
	}

	function startStreaming(): void {
		if (state.value.isStreaming || !config.transportManager) return;

		state.update((current) => ({
			...current,
			isStreaming: true,
			error: null,
		}));

		const timelineUpdatesHandler = config.transportManager?.on(
			'timelineUpdates',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as TimelineUpdatePayload | null;
				const status = mapTimelineObject(payload);
				if (!status) return;
				upsertUnifiedStatus(status, sourceMetadataEnhancer);
			}
		);

		const quoteActivityHandler = config.transportManager?.on(
			'quoteActivity',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as QuoteActivityPayload | null;
				if (!payload) return;

				if (payload.type === 'withdrawn' && payload.quote?.id) {
					removeItemsByPredicate((item) => item.id === payload.quote?.id);
					return;
				}

				if (!payload.quote) return;

				const status = mapTimelineObject(payload.quote);
				if (!status) return;

				upsertUnifiedStatus(status, (metadata) => {
					const base = runMetadataEnhancer(metadata, sourceMetadataEnhancer) ?? metadata;
					const next: LesserTimelineMetadata = {
						...(base ?? {}),
						isQuote: true,
					};

					if (typeof status.quoteCount === 'number') {
						next.quoteCount = status.quoteCount;
					}
					if (typeof status.quoteable === 'boolean') {
						next.quoteable = status.quoteable;
					}
					if (status.quotePermissions) {
						next.quotePermission = status.quotePermissions;
					}

					return next;
				});
			}
		);

		const hashtagActivityHandler = config.transportManager?.on(
			'hashtagActivity',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as HashtagActivityPayload | null;
				if (!payload?.post) return;

				const status = mapTimelineObject(payload.post);
				if (!status) return;

				upsertUnifiedStatus(status, (metadata) => {
					const base = runMetadataEnhancer(metadata, sourceMetadataEnhancer) ?? metadata;
					const next: LesserTimelineMetadata = {
						...(base ?? {}),
					};

					const hashtags = new Set(next.hashtags ?? []);
					hashtags.add(payload.hashtag);
					next.hashtags = Array.from(hashtags);

					return next;
				});
			}
		);

		const listUpdatesHandler = config.transportManager?.on(
			'listUpdates',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as ListUpdatePayload | null;
				if (!payload?.list?.id) return;

				const listId = payload.list.id;

				switch (payload.type) {
					case 'deleted':
						removeItemsByPredicate(() => true);
						break;

					case 'account_removed':
						if (payload.account?.id) {
							removeItemsByPredicate((item) => {
								const status = getStatusFromItem(item);
								return status?.account?.id === payload.account?.id;
							});
						}
						break;

					case 'updated':
						updateItemsMetadata(
							(item) => item.metadata?.lesser?.listMemberships?.includes(listId) ?? false,
							(metadata) => {
								const next: LesserTimelineMetadata = {
									...(metadata ?? {}),
								};

								const titles = {
									...(next.listTitles ?? {}),
									[listId]: payload.list.title,
								};

								next.listTitles = titles;
								return next;
							}
						);
						break;

					case 'account_added':
						if (payload.account?.id) {
							updateItemsMetadata(
								(item) => {
									const status = getStatusFromItem(item);
									return status?.account?.id === payload.account?.id;
								},
								(metadata) => {
									const next: LesserTimelineMetadata = {
										...(metadata ?? {}),
									};

									const memberships = new Set(next.listMemberships ?? []);
									memberships.add(listId);
									next.listMemberships = Array.from(memberships);

									const titles = {
										...(next.listTitles ?? {}),
										[listId]: payload.list.title,
									};
									next.listTitles = titles;

									return next;
								}
							);
						}
						break;
				}
			}
		);

		const relationshipUpdatesHandler = config.transportManager?.on(
			'relationshipUpdates',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as RelationshipUpdatePayload | null;
				if (!payload?.actor?.id) return;

				const actorId = payload.actor.id;

				if (
					payload.type === 'blocked' ||
					payload.type === 'muted' ||
					payload.type === 'unfollowed'
				) {
					removeItemsByPredicate((item) => {
						const status = getStatusFromItem(item);
						return status?.account?.id === actorId;
					});
					return;
				}

				updateItemsMetadata(
					(item) => {
						const status = getStatusFromItem(item);
						return status?.account?.id === actorId;
					},
					(metadata) => ({
						...(metadata ?? {}),
						relationshipStatus: payload.type as LesserTimelineMetadata['relationshipStatus'],
						relationshipUpdatedAt: resolveTimestamp(payload.timestamp),
					})
				);
			}
		);

		const activityStreamHandler = config.transportManager?.on(
			'activityStream',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as LesserStreamingUpdate | null;
				if (!payload) return;

				const mapped = mapLesserStreamingUpdate(payload);
				if (!mapped.success || !mapped.data) return;

				const operation = mapped.data;

				if ('itemType' in operation && operation.itemType === 'status') {
					if (deletionMode === 'tombstone') {
						const tombstoneEdit = buildTombstoneEdit(operation.id, {
							deletedAt: new Date(operation.timestamp).toISOString(),
							formerType: 'status',
						});
						applyStreamingEdit(tombstoneEdit);
					} else {
						deleteItem(operation.id);
					}
					return;
				}

				if ('editType' in operation && 'data' in operation) {
					applyStreamingEdit({
						type: 'replace',
						itemId: operation.id,
						data: operation.data,
						timestamp: operation.timestamp,
					});
					return;
				}

				if ('payload' in operation && operation.payload) {
					upsertUnifiedStatus(operation.payload as UnifiedStatus, sourceMetadataEnhancer);
				}
			}
		);

		// Subscribe to connection events
		const errorHandler = config.transportManager?.on('error', (event) => {
			state.update((current) => ({
				...current,
				error: event.error || new Error('Streaming connection error'),
			}));
		});

		const closeHandler = config.transportManager?.on('close', () => {
			state.update((current) => ({
				...current,
				isStreaming: false,
			}));
		});

		if (timelineUpdatesHandler) streamingUnsubscribers.push(timelineUpdatesHandler);
		if (quoteActivityHandler) streamingUnsubscribers.push(quoteActivityHandler);
		if (hashtagActivityHandler) streamingUnsubscribers.push(hashtagActivityHandler);
		if (listUpdatesHandler) streamingUnsubscribers.push(listUpdatesHandler);
		if (relationshipUpdatesHandler) streamingUnsubscribers.push(relationshipUpdatesHandler);
		if (activityStreamHandler) streamingUnsubscribers.push(activityStreamHandler);
		if (errorHandler) streamingUnsubscribers.push(errorHandler);
		if (closeHandler) streamingUnsubscribers.push(closeHandler);

		// Start the transport connection
		try {
			config.transportManager?.connect();
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isStreaming: false,
			}));
		}
	}

	function stopStreaming(): void {
		state.update((current) => ({
			...current,
			isStreaming: false,
		}));

		// Clean up event subscriptions
		streamingUnsubscribers.forEach((unsubscribe) => unsubscribe());
		streamingUnsubscribers = [];

		// Clear pending updates
		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
			updateDebounceTimer = null;
		}
		pendingUpdates = [];
	}

	function subscribe(callback: (value: TimelineState) => void): () => void {
		return state.subscribe(callback);
	}

	function get(): TimelineState {
		return state.value;
	}

	function destroy(): void {
		stopStreaming();

		// Clear all timers
		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
		}

		// Clear state
		state.update(() => ({
			items: [],
			visibleItems: [],
			totalCount: 0,
			isLoading: false,
			error: null,
			isStreaming: false,
			lastSync: null,
			pageInfo: {
				endCursor: null,
				hasNextPage: true,
			},
			virtualWindow: {
				startIndex: 0,
				endIndex: 0,
				itemHeight: config.itemHeight || 50,
				containerHeight: config.containerHeight || 400,
			},
		}));
	}

	function generateId(): string {
		return `timeline_${globalThis.crypto.randomUUID()}`;
	}

	// Lesser-specific filters and selectors
	function getItemsWithCost(maxCost?: number): TimelineItem[] {
		return state.value.items.filter((item) => {
			const cost = item.metadata?.lesser?.estimatedCost;
			if (cost === undefined) return false;
			return maxCost === undefined || cost <= maxCost;
		});
	}

	function getItemsWithTrustScore(minScore: number): TimelineItem[] {
		return state.value.items.filter((item) => {
			const score = item.metadata?.lesser?.authorTrustScore;
			return score !== undefined && score >= minScore;
		});
	}

	function getItemsWithCommunityNotes(): TimelineItem[] {
		return state.value.items.filter((item) => item.metadata?.lesser?.hasCommunityNotes === true);
	}

	function getQuotePosts(): TimelineItem[] {
		return state.value.items.filter((item) => item.metadata?.lesser?.isQuote === true);
	}

	function getModeratedItems(action?: string): TimelineItem[] {
		return state.value.items.filter((item) => {
			const moderationAction = item.metadata?.lesser?.aiModerationAction;
			if (moderationAction === undefined || moderationAction === 'NONE') return false;
			return action === undefined || moderationAction === action;
		});
	}

	return {
		subscribe,
		destroy,
		get,
		addItem,
		replaceItem,
		deleteItem,
		deleteStatus,
		applyStreamingEdit,
		updateVirtualWindow,
		loadMore,
		refresh,
		startStreaming,
		stopStreaming,
		// Lesser-specific methods
		getItemsWithCost,
		getItemsWithTrustScore,
		getItemsWithCommunityNotes,
		getQuotePosts,
		getModeratedItems,
	};
}
