/**
 * Unified Model to Timeline Item Converter
 *
 * Converts UnifiedStatus to TimelineItem with Lesser metadata properly populated
 */

import type { UnifiedStatus } from '../models/unified.js';
import type { TimelineItem, LesserTimelineMetadata } from './types.js';

/**
 * Convert UnifiedStatus to TimelineItem with Lesser metadata
 */
export function unifiedStatusToTimelineItem(
	status: UnifiedStatus
): Omit<TimelineItem, 'id' | 'timestamp'> {
	const lesserMetadata: LesserTimelineMetadata = {};

	if (status.isDeleted) {
		lesserMetadata.isDeleted = true;
	}

	if (status.deletedAt) {
		lesserMetadata.deletedAt = status.deletedAt;
	}

	if (status.formerType) {
		lesserMetadata.formerType = status.formerType;
	}

	// Populate Lesser metadata from UnifiedStatus fields
	if (status.estimatedCost !== undefined) {
		lesserMetadata.estimatedCost = status.estimatedCost;
	}

	if (status.moderationScore !== undefined) {
		lesserMetadata.moderationScore = status.moderationScore;
	}

	if (status.communityNotes && status.communityNotes.length > 0) {
		lesserMetadata.hasCommunityNotes = true;
		lesserMetadata.communityNotesCount = status.communityNotes.length;
	}

	if (status.quoteUrl) {
		lesserMetadata.isQuote = true;
	}

	if (status.quoteCount !== undefined) {
		lesserMetadata.quoteCount = status.quoteCount;
	}

	if (status.quoteable !== undefined) {
		lesserMetadata.quoteable = status.quoteable;
	}

	if (status.quotePermissions) {
		lesserMetadata.quotePermission = status.quotePermissions;
	}

	if (status.account.trustScore !== undefined) {
		lesserMetadata.authorTrustScore = status.account.trustScore;
	}

	if (status.aiAnalysis) {
		lesserMetadata.aiModerationAction = status.aiAnalysis.moderationAction;
		lesserMetadata.aiConfidence = status.aiAnalysis.confidence;
	}

	return {
		type: status.isDeleted ? 'tombstone' : 'status',
		content: status,
		metadata:
			Object.keys(lesserMetadata).length > 0
				? {
						lesser: lesserMetadata,
					}
				: undefined,
	};
}

/**
 * Batch convert multiple UnifiedStatus objects to TimelineItems
 */
export function unifiedStatusesToTimelineItems(
	statuses: UnifiedStatus[]
): Array<Omit<TimelineItem, 'id' | 'timestamp'>> {
	return statuses.map(unifiedStatusToTimelineItem);
}
