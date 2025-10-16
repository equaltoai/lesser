#!/usr/bin/env python3
"""
split_schema_resolvers.py
=========================

Utility script to help reorganize the massive `graph/schema.resolvers.go`
file into domain-specific resolver files.  The script operates in preview
mode by default: it prints the proposed file contents to stdout without
writing anything to disk.  Pass `--apply` to write the generated files into
the `graph/` directory.

The script intentionally only moves resolver methods belonging to the
`queryResolver`, `mutationResolver`, and `subscriptionResolver` types.  Any
helper methods or other receiver types remain in `schema.resolvers.go` for
manual processing.

Usage::

    python scripts/split_schema_resolvers.py [--apply]

"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys
from collections import defaultdict
from dataclasses import dataclass
from typing import Dict, Iterable, List, Optional, Tuple


WORKSPACE_ROOT = pathlib.Path(__file__).resolve().parents[1]
GRAPH_DIR = WORKSPACE_ROOT / "graph"
SCHEMA_RESOLVERS_PATH = GRAPH_DIR / "schema.resolvers.go"


# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ResolverFunction:
    receiver: str
    name: str
    source: str
    start_line: int
    end_line: int


# ---------------------------------------------------------------------------
# Resolver grouping configuration
# ---------------------------------------------------------------------------


QUERY_GROUPS: Dict[str, str] = {
    # Notes & timelines
    "Object": "query_resolvers_notes.go",
    "Timeline": "query_resolvers_notes.go",
    "ThreadContext": "query_resolvers_notes.go",
    "Search": "query_resolvers_notes.go",

    # Hashtags
    "Hashtag": "query_resolvers_hashtags.go",
    "HashtagTimeline": "query_resolvers_hashtags.go",
    "MultiHashtagTimeline": "query_resolvers_hashtags.go",
    "SuggestedHashtags": "query_resolvers_hashtags.go",
    "FollowedHashtags": "query_resolvers_hashtags.go",

    # Accounts & profiles
    "Actor": "query_resolvers_accounts.go",
    "ProfileDirectory": "query_resolvers_accounts.go",
    "Suggestions": "query_resolvers_accounts.go",
    "RemoveSuggestion": "query_resolvers_accounts.go",
    "CustomEmojis": "query_resolvers_accounts.go",

    # Relationships & trust
    "Relationship": "query_resolvers_relationships.go",
    "Relationships": "query_resolvers_relationships.go",
    "TrustGraph": "query_resolvers_relationships.go",

    # Lists
    "Lists": "query_resolvers_lists.go",
    "List": "query_resolvers_lists.go",
    "ListAccounts": "query_resolvers_lists.go",

    # Notifications & conversations
    "Notifications": "query_resolvers_notifications.go",
    "Conversations": "query_resolvers_conversations.go",
    "Conversation": "query_resolvers_conversations.go",

    # Media & streaming
    "Media": "query_resolvers_media.go",
    "MediaStreamURL": "query_resolvers_media.go",
    "StreamingAnalytics": "query_resolvers_media.go",
    "SupportedBitrates": "query_resolvers_media.go",
    "PopularStreams": "query_resolvers_media.go",

    # Federation
    "FederationStatus": "query_resolvers_federation.go",
    "FederationCosts": "query_resolvers_federation.go",
    "FederationHealth": "query_resolvers_federation.go",
    "FederationFlow": "query_resolvers_federation.go",
    "FederationLimits": "query_resolvers_federation.go",
    "FederationMap": "query_resolvers_federation.go",
    "InstanceMetrics": "query_resolvers_federation.go",
    "InstanceHealthReport": "query_resolvers_federation.go",
    "InstanceBudgets": "query_resolvers_federation.go",
    "InstanceRelationships": "query_resolvers_federation.go",
    "SeveredRelationships": "query_resolvers_federation.go",
    "AffectedRelationships": "query_resolvers_federation.go",

    # Moderation
    "ModerationDashboard": "query_resolvers_moderation.go",
    "ModerationQueue": "query_resolvers_moderation.go",
    "ModerationPatterns": "query_resolvers_moderation.go",
    "ModerationEffectiveness": "query_resolvers_moderation.go",
    "PatternEffectiveness": "query_resolvers_moderation.go",
    "ModeratorActivity": "query_resolvers_moderation.go",

    # Cost & performance
    "CostBreakdown": "query_resolvers_cost.go",
    "CostProjections": "query_resolvers_cost.go",
    "BandwidthUsage": "query_resolvers_cost.go",
    "PerformanceMetrics": "query_resolvers_cost.go",
    "SlowQueries": "query_resolvers_cost.go",
    "InfrastructureHealth": "query_resolvers_cost.go",

    # AI / analysis
    "AiAnalysis": "query_resolvers_ai.go",
    "AiCapabilities": "query_resolvers_ai.go",
    "AiStats": "query_resolvers_ai.go",
    "ExplainObject": "query_resolvers_ai.go",

    # Admin / scheduling
    "ScheduledStatuses": "query_resolvers_admin.go",
    "ScheduledStatus": "query_resolvers_admin.go",
}


MUTATION_GROUPS: Dict[str, str] = {
    # Notes & scheduling
    "CreateNote": "mutation_resolvers_notes.go",
    "CreateQuoteNote": "mutation_resolvers_notes.go",
    "DeleteObject": "mutation_resolvers_notes.go",
    "ScheduleStatus": "mutation_resolvers_notes.go",
    "CancelScheduledStatus": "mutation_resolvers_notes.go",
    "UpdateScheduledStatus": "mutation_resolvers_notes.go",

    # Relationships & social graph
    "FollowActor": "mutation_resolvers_relationships.go",
    "UnfollowActor": "mutation_resolvers_relationships.go",
    "BlockActor": "mutation_resolvers_relationships.go",
    "UnblockActor": "mutation_resolvers_relationships.go",
    "MuteActor": "mutation_resolvers_relationships.go",
    "UnmuteActor": "mutation_resolvers_relationships.go",
    "FollowHashtag": "mutation_resolvers_relationships.go",
    "UnfollowHashtag": "mutation_resolvers_relationships.go",
    "MuteHashtag": "mutation_resolvers_relationships.go",
    "UpdateHashtagNotifications": "mutation_resolvers_relationships.go",

    # Engagement actions
    "LikeObject": "mutation_resolvers_engagement.go",
    "UnlikeObject": "mutation_resolvers_engagement.go",
    "BookmarkObject": "mutation_resolvers_engagement.go",
    "UnbookmarkObject": "mutation_resolvers_engagement.go",
    "ShareObject": "mutation_resolvers_engagement.go",
    "UnshareObject": "mutation_resolvers_engagement.go",
    "PinObject": "mutation_resolvers_engagement.go",
    "UnpinObject": "mutation_resolvers_engagement.go",

    # Lists
    "CreateList": "mutation_resolvers_lists.go",
    "UpdateList": "mutation_resolvers_lists.go",
    "DeleteList": "mutation_resolvers_lists.go",
    "AddAccountsToList": "mutation_resolvers_lists.go",
    "RemoveAccountsFromList": "mutation_resolvers_lists.go",

    # Moderation
    "CreateModerationPattern": "mutation_resolvers_moderation.go",
    "UpdateModerationPattern": "mutation_resolvers_moderation.go",
    "DeleteModerationPattern": "mutation_resolvers_moderation.go",
    "TrainModerationModel": "mutation_resolvers_moderation.go",
    "FlagObject": "mutation_resolvers_moderation.go",
    "VoteCommunityNote": "mutation_resolvers_moderation.go",
    "AddCommunityNote": "mutation_resolvers_moderation.go",

    # Federation controls
    "PauseFederation": "mutation_resolvers_federation.go",
    "ResumeFederation": "mutation_resolvers_federation.go",
    "SetFederationLimit": "mutation_resolvers_federation.go",
    "SetInstanceBudget": "mutation_resolvers_federation.go",
    "OptimizeFederationCosts": "mutation_resolvers_federation.go",
    "AcknowledgeSeverance": "mutation_resolvers_federation.go",
    "AttemptReconnection": "mutation_resolvers_federation.go",

    # Media
    "UpdateMedia": "mutation_resolvers_media.go",
    "RequestStreamingURL": "mutation_resolvers_media.go",
    "PreloadMedia": "mutation_resolvers_media.go",
    "ReportStreamingQuality": "mutation_resolvers_media.go",

    # Conversations & notifications
    "MarkConversationAsRead": "mutation_resolvers_conversations.go",
    "DeleteConversation": "mutation_resolvers_conversations.go",
    "DismissNotification": "mutation_resolvers_notifications.go",
    "ClearNotifications": "mutation_resolvers_notifications.go",

    # Emojis & misc
    "CreateEmoji": "mutation_resolvers_emoji.go",
    "UpdateEmoji": "mutation_resolvers_emoji.go",
    "DeleteEmoji": "mutation_resolvers_emoji.go",
}


SUBSCRIPTION_GROUPS: Dict[str, str] = {
    # Timelines & activity streams
    "TimelineUpdates": "subscription_resolvers_timelines.go",
    "ActivityStream": "subscription_resolvers_timelines.go",

    # Relationships & trust graph
    "RelationshipUpdates": "subscription_resolvers_relationships.go",
    "TrustUpdates": "subscription_resolvers_relationships.go",

    # Conversations & notifications
    "NotificationStream": "subscription_resolvers_notifications.go",
    "ConversationUpdates": "subscription_resolvers_conversations.go",

    # Federation & infrastructure monitoring
    "FederationHealthUpdates": "subscription_resolvers_federation.go",
    "InfrastructureEvent": "subscription_resolvers_federation.go",

    # Cost & performance monitoring
    "CostAlerts": "subscription_resolvers_cost.go",
    "BudgetAlerts": "subscription_resolvers_cost.go",
    "CostUpdates": "subscription_resolvers_cost.go",
    "MetricsUpdates": "subscription_resolvers_cost.go",
    "PerformanceAlert": "subscription_resolvers_cost.go",

    # Moderation streams
    "ModerationAlerts": "subscription_resolvers_moderation.go",
    "ModerationQueueUpdate": "subscription_resolvers_moderation.go",
    "ModerationEvents": "subscription_resolvers_moderation.go",

    # AI & threat intelligence
    "AiAnalysisUpdates": "subscription_resolvers_ai.go",
    "ThreatIntelligence": "subscription_resolvers_ai.go",

    # Lists & hashtags
    "ListUpdates": "subscription_resolvers_lists.go",
    "HashtagActivity": "subscription_resolvers_hashtags.go",
    "QuoteActivity": "subscription_resolvers_quotes.go",
}


# ---------------------------------------------------------------------------
# Source parsing helpers
# ---------------------------------------------------------------------------


FUNC_PATTERN = re.compile(
    r"func\s+\(r\s+\*(?P<receiver>queryResolver|mutationResolver|subscriptionResolver)\)\s+"
    r"(?P<name>[A-Za-z0-9_]+)",
    re.MULTILINE,
)


def _find_open_brace(source: str, start_idx: int) -> int:
    i = start_idx
    in_string: Optional[str] = None
    in_block_comment = False
    in_line_comment = False

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
            if in_string == '"':
                if ch == "\\":
                    i += 1
                elif ch == '"':
                    in_string = None
            elif in_string == "'":
                if ch == "\\":
                    i += 1
                elif ch == "'":
                    in_string = None
            elif in_string == "`":
                if ch == "`":
                    in_string = None
        else:
            if ch == '/' and nxt == '/':
                in_line_comment = True
                i += 1
            elif ch == '/' and nxt == '*':
                in_block_comment = True
                i += 1
            elif ch in ('"', "'", '`'):
                in_string = ch
            elif ch == '{':
                return i

        i += 1

    return -1


def _find_matching_brace(source: str, open_idx: int) -> int:
    assert source[open_idx] == '{'
    depth = 0
    i = open_idx
    in_string: Optional[str] = None
    in_block_comment = False
    in_line_comment = False

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
            if in_string == '"':
                if ch == "\\":
                    i += 1
                elif ch == '"':
                    in_string = None
            elif in_string == "'":
                if ch == "\\":
                    i += 1
                elif ch == "'":
                    in_string = None
            elif in_string == "`":
                if ch == "`":
                    in_string = None
        else:
            if ch == '/' and nxt == '/':
                in_line_comment = True
                i += 1
            elif ch == '/' and nxt == '*':
                in_block_comment = True
                i += 1
            elif ch in ('"', "'", '`'):
                in_string = ch
            elif ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
                if depth == 0:
                    return i

        i += 1

    raise ValueError("Failed to find matching closing brace")


def _include_doc_comments(source: str, func_start: int) -> int:
    idx = func_start
    while idx > 0:
        prev_newline = source.rfind("\n", 0, idx)
        if prev_newline == -1:
            return 0
        line = source[prev_newline + 1:idx].strip()
        if line.startswith("//"):
            idx = prev_newline
            continue
        if line == "":
            idx = prev_newline
            continue
        break
    return idx


def parse_resolvers(source: str) -> List[ResolverFunction]:
    functions: List[ResolverFunction] = []

    for match in FUNC_PATTERN.finditer(source):
        receiver = match.group("receiver")
        name = match.group("name")

        open_brace = _find_open_brace(source, match.end())
        if open_brace == -1:
            raise ValueError(f"Could not locate function body for {receiver}.{name}")

        close_brace = _find_matching_brace(source, open_brace)

        snippet_start = _include_doc_comments(source, match.start())
        snippet = source[snippet_start:close_brace + 1]

        start_line = source.count("\n", 0, snippet_start) + 1
        end_line = source.count("\n", 0, close_brace + 1)

        functions.append(
            ResolverFunction(
                receiver=receiver,
                name=name,
                source=snippet.strip() + "\n",
                start_line=start_line,
                end_line=end_line,
            )
        )

    return functions


def group_function(func: ResolverFunction) -> Optional[str]:
    if func.receiver == "queryResolver":
        return QUERY_GROUPS.get(func.name)
    if func.receiver == "mutationResolver":
        return MUTATION_GROUPS.get(func.name)
    if func.receiver == "subscriptionResolver":
        return SUBSCRIPTION_GROUPS.get(func.name)
    return None


def build_file_header(filename: str) -> str:
    comment = (
        "// NOTE: imports intentionally omitted. Run gofmt/goimports and add any\n"
        "// required imports after generating these files.\n"
    )
    return f"package graph\n\n{comment}\n"


def render_functions(funcs: Iterable[ResolverFunction]) -> str:
    return "\n".join(func.source for func in funcs)


def split_functions(functions: List[ResolverFunction]) -> Tuple[Dict[str, List[ResolverFunction]], List[ResolverFunction]]:
    grouped: Dict[str, List[ResolverFunction]] = defaultdict(list)
    leftovers: List[ResolverFunction] = []

    for func in functions:
        target = group_function(func)
        if target:
            grouped[target].append(func)
        else:
            leftovers.append(func)

    return grouped, leftovers


def write_files(grouped: Dict[str, List[ResolverFunction]], apply: bool, show_content: bool) -> None:
    for filename, funcs in sorted(grouped.items()):
        if not funcs:
            continue
        header = build_file_header(filename)
        content = header + render_functions(funcs)

        print(f"=== {filename} ({len(funcs)} resolvers) ===")
        if show_content:
            print(content)

        if apply:
            output_path = GRAPH_DIR / filename
            output_path.write_text(content)
            print(f"[wrote] {output_path.relative_to(WORKSPACE_ROOT)}")


def main(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description="Split schema.resolvers.go into domain-specific files")
    parser.add_argument(
        "--apply",
        action="store_true",
        help="Write generated files to disk"
    )
    parser.add_argument(
        "--full",
        action="store_true",
        help="Show full generated file content instead of just listing filenames"
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Show detailed lists of helpers that remain in schema.resolvers.go"
    )
    args = parser.parse_args(argv)

    if not SCHEMA_RESOLVERS_PATH.exists():
        print(f"Resolver file not found: {SCHEMA_RESOLVERS_PATH}", file=sys.stderr)
        return 1

    source = SCHEMA_RESOLVERS_PATH.read_text()
    functions = parse_resolvers(source)

    grouped, leftovers = split_functions(functions)
    write_files(grouped, apply=args.apply, show_content=args.full)

    total_grouped = sum(len(fns) for fns in grouped.values())

    print("\n--- Summary ---")
    print(f"Total resolver methods parsed: {len(functions)}")
    print(f"Resolvers assigned to files: {total_grouped}")
    print(f"Resolvers left in schema.resolvers.go: {len(leftovers)}")

    if leftovers:
        print("\nUnassigned resolvers remain in schema.resolvers.go")
        if args.verbose or args.full:
            print("Listing helpers/functions to relocate:")
            for func in leftovers:
                print(f"  {func.receiver}.{func.name} (lines {func.start_line}-{func.end_line})")
        else:
            print("(run with --verbose for detailed list)")

    if not args.apply:
        print("\nPreview mode. Re-run with --apply to write files.")

    return 0


if __name__ == "__main__":
    sys.exit(main())


