#!/usr/bin/env python3
"""
Expanded stage-aware read validation for the Lesser GraphQL API.

Environment variables:
  GRAPHQL_STAGE            -> deployment stage (default: dev)
  GRAPHQL_DOMAIN           -> root domain (default: lesser.host)
  GRAPHQL_ENDPOINT         -> full override of GraphQL URL
  GRAPHQL_TOKEN            -> optional Bearer token (recommended)
  GRAPHQL_HASHTAG          -> hashtag to exercise (default: lesser)
  GRAPHQL_SEARCH_QUERY     -> text for search (default: "lesser")
  GRAPHQL_THREAD_ROOT      -> conversation ID for conversation/threadContext
  GRAPHQL_LIST_ID          -> list ID for listAccounts validation
"""

import json
import os
from typing import Dict, Optional

import requests

BASE_DOMAIN = os.getenv("GRAPHQL_DOMAIN", "lesser.host")
STAGE = os.getenv("GRAPHQL_STAGE", "dev")
DEFAULT_ENDPOINT = f"https://{STAGE}.{BASE_DOMAIN}/api/graphql"
GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", DEFAULT_ENDPOINT)
AUTH_TOKEN = os.getenv("GRAPHQL_TOKEN")
DEFAULT_HASHTAG = os.getenv("GRAPHQL_HASHTAG", "lesser")
SEARCH_QUERY = os.getenv("GRAPHQL_SEARCH_QUERY", "lesser")
THREAD_ROOT = os.getenv("GRAPHQL_THREAD_ROOT")
LIST_ID = os.getenv("GRAPHQL_LIST_ID")


def headers() -> Dict[str, str]:
    hdrs: Dict[str, str] = {"Content-Type": "application/json"}
    if AUTH_TOKEN:
        hdrs["Authorization"] = f"Bearer {AUTH_TOKEN}"
    return hdrs


def run_query(
    title: str,
    query: str,
    variables: Optional[Dict[str, object]] = None,
    allow_errors: bool = False,
) -> None:
    print(f"\n=== {title} ===")
    payload = {"query": query}
    if variables:
        payload["variables"] = variables

    response = requests.post(
        GRAPHQL_ENDPOINT,
        json=payload,
        headers=headers(),
        timeout=30,
    )
    print(f"Status: {response.status_code}")
    try:
        body = response.json()
    except ValueError:
        print(f"Non-JSON response: {response.text}")
        response.raise_for_status()
        return

    print(json.dumps(body, indent=2))
    response.raise_for_status()
    if "errors" in body:
        if allow_errors:
            print(f"[warn] GraphQL errors encountered in {title}: {body['errors']}")
        else:
            raise SystemExit(f"GraphQL errors detected: {body['errors']}")


def validate_timelines() -> None:
    run_query(
        "Home Timeline",
        """
        query HomeTimeline {
            timeline(type: HOME, first: 5) {
                edges {
                    cursor
                    node {
                        id
                        content
                        createdAt
                        visibility
                        actor { username }
                    }
                }
                pageInfo {
                    hasNextPage
                    endCursor
                }
            }
        }
        """,
    )

    run_query(
        "Public Timeline",
        """
        query PublicTimeline {
            timeline(type: PUBLIC, first: 5) {
                edges {
                    cursor
                    node {
                        id
                        content
                        createdAt
                        visibility
                        actor { username }
                    }
                }
                pageInfo {
                    hasNextPage
                    endCursor
                }
            }
        }
        """,
    )


def validate_hashtag_timeline() -> None:
    run_query(
        "Hashtag Timeline",
        """
        query HashtagTimeline($tag: String!, $first: Int) {
            hashtagTimeline(hashtag: $tag, first: $first) {
                edges {
                    cursor
                    node {
                        id
                        content
                        createdAt
                        actor { username }
                    }
                }
                pageInfo {
                    hasNextPage
                    endCursor
                }
            }
        }
        """,
        variables={"tag": DEFAULT_HASHTAG, "first": 5},
    )


def validate_search() -> None:
    run_query(
        "Search Statuses",
        """
        query SearchStatuses($query: String!) {
            search(query: $query, type: "STATUS", first: 5) {
                accounts { username }
                statuses {
                    id
                    content
                    actor { username }
                }
                hashtags {
                    name
                }
            }
        }
        """,
        variables={"query": SEARCH_QUERY},
    )


def validate_notifications() -> None:
    run_query(
        "Notifications",
        """
        query Notifications {
            notifications(first: 5) {
                edges {
                    cursor
                    node {
                        id
                        type
                        createdAt
                        status { id }
                        account { username }
                    }
                }
                pageInfo {
                    hasNextPage
                    endCursor
                }
            }
        }
        """,
        allow_errors=True,
    )


def validate_conversations() -> None:
    run_query(
        "Conversations",
        """
        query Conversations {
            conversations(first: 5) {
                id
                unread
                lastStatus { id content }
                accounts { username }
                createdAt
                updatedAt
            }
        }
        """,
        allow_errors=True,
    )

    if THREAD_ROOT:
        run_query(
            "Conversation Detail",
            """
            query ConversationDetail($id: ID!) {
                conversation(id: $id) {
                    id
                    lastStatus { id content }
                    accounts { username }
                }
            }
            """,
            variables={"id": THREAD_ROOT},
            allow_errors=True,
        )

        run_query(
            "Thread Context",
            """
            query ThreadContext($noteId: ID!) {
                threadContext(noteId: $noteId) {
                    rootNote { id content }
                    replyCount
                    participantCount
                    missingPosts
                    syncStatus
                    lastActivity
                }
            }
            """,
            variables={"noteId": THREAD_ROOT},
            allow_errors=True,
        )


def validate_lists() -> None:
    run_query(
        "Lists",
        """
        query Lists {
            lists {
                id
                title
                repliesPolicy
            }
        }
        """,
    )

    if LIST_ID:
        run_query(
            "List Accounts",
            """
        query ListAccounts($id: ID!) {
            listAccounts(id: $id) {
                id
                username
                displayName
            }
        }
            """,
            variables={"id": LIST_ID},
        )


def main() -> None:
    print(f"Validating GraphQL reads at: {GRAPHQL_ENDPOINT} (stage={STAGE})")

    validate_timelines()
    validate_hashtag_timeline()
    validate_search()
    validate_notifications()
    validate_conversations()
    validate_lists()


if __name__ == "__main__":
    try:
        main()
    except requests.exceptions.ConnectionError:
        print(f"\nError: Could not connect to GraphQL endpoint at {GRAPHQL_ENDPOINT}")
        raise
