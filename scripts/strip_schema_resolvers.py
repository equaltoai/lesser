#!/usr/bin/env python3
"""Remove resolver definitions from schema.resolvers.go that were split out."""

from __future__ import annotations

import pathlib
import re
from typing import List, Tuple

SCHEMA_PATH = pathlib.Path("graph/schema.resolvers.go")

# Map receiver type to method names we already moved into separate files
REMOVED_METHODS = {
    "queryResolver": [
        "Actor", "Object", "Timeline", "Hashtag", "HashtagTimeline",
        "MultiHashtagTimeline", "SuggestedHashtags", "FollowedHashtags",
        "Search", "List", "Lists", "ListAccounts", "Media",
        "MediaStreamURL", "StreamingAnalytics", "SupportedBitrates",
        "Notifications", "Conversations", "Conversation",
        "Relationship", "Relationships", "ProfileDirectory",
        "Suggestions", "RemoveSuggestion", "CostBreakdown",
        "CostProjections", "BandwidthUsage", "PerformanceMetrics",
        "SlowQueries", "InfrastructureHealth", "FederationStatus",
        "FederationCosts", "FederationHealth", "FederationFlow",
        "FederationLimits", "FederationMap", "InstanceMetrics",
        "InstanceRelationships", "InstanceBudgets",
        "InstanceHealthReport", "ModerationDashboard",
        "ModerationQueue", "ModerationPatterns",
        "ModerationEffectiveness", "PatternEffectiveness",
        "ModeratorActivity", "AiAnalysis", "AiCapabilities",
        "AiStats", "ExplainObject", "ScheduledStatuses",
        "ScheduledStatus", "ThreadContext", "PopularStreams",
    ],
    "mutationResolver": [
        "CreateNote", "CreateQuoteNote", "DeleteObject", "LikeObject",
        "UnlikeObject", "ShareObject", "UnshareObject", "BookmarkObject",
        "UnbookmarkObject", "PinObject", "UnpinObject", "FollowActor",
        "UnfollowActor", "BlockActor", "UnblockActor", "MuteActor",
        "UnmuteActor", "FollowHashtag", "UnfollowHashtag",
        "MuteHashtag", "UpdateHashtagNotifications", "CreateList",
        "UpdateList", "DeleteList", "AddAccountsToList",
        "RemoveAccountsFromList", "CreateEmoji", "UpdateEmoji",
        "DeleteEmoji", "ScheduleStatus", "CancelScheduledStatus",
        "UpdateScheduledStatus", "MarkConversationAsRead",
        "DeleteConversation", "DismissNotification",
        "ClearNotifications", "CreateModerationPattern",
        "UpdateModerationPattern", "DeleteModerationPattern",
        "TrainModerationModel", "FlagObject", "VoteCommunityNote",
        "AddCommunityNote", "PauseFederation", "ResumeFederation",
        "SetFederationLimit", "SetInstanceBudget",
        "OptimizeFederationCosts", "AcknowledgeSeverance",
        "AttemptReconnection", "UpdateMedia", "RequestStreamingURL",
        "PreloadMedia", "ReportStreamingQuality",
        "UpdateStreamingPreferences", "WithdrawFromQuotes",
        "UpdateQuotePermissions", "SyncThread", "SyncMissingReplies",
    ],
    "subscriptionResolver": [
        "TimelineUpdates", "ActivityStream", "NotificationStream",
        "ConversationUpdates", "ListUpdates", "RelationshipUpdates",
        "TrustUpdates", "ModerationAlerts", "ModerationEvents",
        "ModerationQueueUpdate", "CostAlerts", "BudgetAlerts",
        "CostUpdates", "FederationHealthUpdates", "PerformanceAlert",
        "ModerationQueue", "MetricsUpdates", "InfrastructureEvent",
        "AiAnalysisUpdates", "ThreatIntelligence", "HashtagActivity",
        "QuoteActivity",
    ],
}


def read_schema() -> str:
    if not SCHEMA_PATH.exists():
        raise FileNotFoundError(f"Resolver file not found: {SCHEMA_PATH}")
    return SCHEMA_PATH.read_text()


def find_function_spans(source: str, receiver: str, name: str) -> List[Tuple[int, int]]:
    pattern = re.compile(rf"func \(r \*{re.escape(receiver)}\)\s+{re.escape(name)}\s*\(")
    spans = []
    for match in pattern.finditer(source):
        open_brace = _find_open_brace(source, match.end())
        if open_brace == -1:
            continue
        close_brace = _find_matching_brace(source, open_brace)
        spans.append((match.start(), close_brace + 1))
    return spans


def _find_open_brace(source: str, start_idx: int) -> int:
    i = start_idx
    in_string = None
    in_line_comment = False
    in_block_comment = False
    while i < len(source):
        ch = source[i]
        nxt = source[i + 1] if i + 1 < len(source) else ""
        if in_line_comment:
            if ch == "\n":
                in_line_comment = False
        elif in_block_comment:
            if ch == "*" and nxt == "/":
                in_block_comment = False
                i += 1
        elif in_string:
            if ch == "\\" and in_string != "`":
                i += 1
            elif ch == in_string:
                in_string = None
        else:
            if ch == "/" and nxt == "/":
                in_line_comment = True
                i += 1
            elif ch == "/" and nxt == "*":
                in_block_comment = True
                i += 1
            elif ch in ('"', "'", "`"):
                in_string = ch
            elif ch == "{":
                return i
        i += 1
    return -1


def _find_matching_brace(source: str, open_idx: int) -> int:
    depth = 0
    i = open_idx
    in_string = None
    in_line_comment = False
    in_block_comment = False
    while i < len(source):
        ch = source[i]
        nxt = source[i + 1] if i + 1 < len(source) else ""
        if in_line_comment:
            if ch == "\n":
                in_line_comment = False
        elif in_block_comment:
            if ch == "*" and nxt == "/":
                in_block_comment = False
                i += 1
        elif in_string:
            if ch == "\\" and in_string != "`":
                i += 1
            elif ch == in_string:
                in_string = None
        else:
            if ch == "/" and nxt == "/":
                in_line_comment = True
                i += 1
            elif ch == "/" and nxt == "*":
                in_block_comment = True
                i += 1
            elif ch in ('"', "'", "`"):
                in_string = ch
            elif ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    return i
        i += 1
    return len(source) - 1


def remove_resolvers(source: str) -> Tuple[str, List[str]]:
    spans_to_remove: List[Tuple[int, int]] = []
    removed: List[str] = []
    for receiver, names in REMOVED_METHODS.items():
        for name in names:
            spans = find_function_spans(source, receiver, name)
            for span in spans:
                spans_to_remove.append(span)
                removed.append(f"{receiver}.{name}")
    if not spans_to_remove:
        return source, []
    spans_to_remove.sort()
    new_source_parts = []
    last_idx = 0
    for start, end in spans_to_remove:
        if start < last_idx:
            continue
        new_source_parts.append(source[last_idx:start])
        last_idx = end
    new_source_parts.append(source[last_idx:])
    return "".join(new_source_parts), removed


def main() -> None:
    source = read_schema()
    new_source, removed = remove_resolvers(source)
    if not removed:
        print("No resolver methods removed; schema may already be stripped.")
        return
    SCHEMA_PATH.write_text(new_source)
    print(f"Removed {len(removed)} resolver methods from schema.resolvers.go")


if __name__ == "__main__":
    main()


