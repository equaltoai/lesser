/**
 * Unified Model to Timeline Item Converter
 *
 * Converts UnifiedStatus to TimelineItem with Lesser metadata properly populated
 */
import type { UnifiedStatus } from '../models/unified.js';
import type { TimelineItem } from './types.js';
/**
 * Convert UnifiedStatus to TimelineItem with Lesser metadata
 */
export declare function unifiedStatusToTimelineItem(
	status: UnifiedStatus
): Omit<TimelineItem, 'id' | 'timestamp'>;
/**
 * Batch convert multiple UnifiedStatus objects to TimelineItems
 */
export declare function unifiedStatusesToTimelineItems(
	statuses: UnifiedStatus[]
): Array<Omit<TimelineItem, 'id' | 'timestamp'>>;
//# sourceMappingURL=unifiedToTimeline.d.ts.map
