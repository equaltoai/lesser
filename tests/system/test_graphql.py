#!/usr/bin/env python3
"""
Stage-aware validation script for the Lesser GraphQL API.

Environment variables:
  GRAPHQL_STAGE           -> deployment stage (default: dev)
  GRAPHQL_DOMAIN          -> root domain (default: lesser.host)
  GRAPHQL_ENDPOINT        -> full override of GraphQL URL
  GRAPHQL_ACTOR_USERNAME  -> username to use for actor query (default: admin)
  GRAPHQL_TOKEN           -> optional Bearer token for authenticated tests
"""

import json
import os
import time
from typing import Dict

import requests

BASE_DOMAIN = os.getenv("GRAPHQL_DOMAIN", "lesser.host")
STAGE = os.getenv("GRAPHQL_STAGE", "dev")
DEFAULT_ENDPOINT = f"https://{STAGE}.{BASE_DOMAIN}/api/graphql"
GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", DEFAULT_ENDPOINT)
ACTOR_USERNAME = os.getenv("GRAPHQL_ACTOR_USERNAME", "admin")
AUTH_TOKEN = os.getenv("GRAPHQL_TOKEN")


def auth_header_value(token: str) -> str:
    token = token.strip()
    if token.lower().startswith("bearer "):
        return token
    return f"Bearer {token}"


def build_headers() -> Dict[str, str]:
    headers: Dict[str, str] = {"Content-Type": "application/json"}
    if AUTH_TOKEN:
        headers["Authorization"] = auth_header_value(AUTH_TOKEN)
    return headers


def post_graphql(payload: Dict[str, object]) -> requests.Response:
    max_attempts = int(os.getenv("GRAPHQL_RETRY_ATTEMPTS", "3"))
    delay = float(os.getenv("GRAPHQL_RETRY_DELAY", "2"))

    for attempt in range(1, max_attempts + 1):
        response = requests.post(
            GRAPHQL_ENDPOINT,
            json=payload,
            headers=build_headers(),
            timeout=30,
        )
        if response.status_code < 500 or attempt == max_attempts:
            return response
        print(
            f"[warn] GraphQL request retry {attempt}/{max_attempts} -- status {response.status_code}; sleeping {delay}s"
        )
        time.sleep(delay)

    return response


def test_instance_metrics() -> None:
    """Test the instanceMetrics query."""
    query = """
    query GetInstanceMetrics {
        instanceMetrics {
            activeUsers
            requestsPerMinute
            averageLatencyMs
            storageUsedGB
            estimatedMonthlyCost
            lastUpdated
        }
    }
    """

    response = post_graphql({"query": query})
    print("\n=== Instance Metrics Query ===")
    print(f"Status: {response.status_code}")
    print(f"Headers: {dict(response.headers)}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")

    if "X-Cost-Total-Micros" in response.headers:
        print("\nCost Information:")
        print(f"  Total Cost: {response.headers.get('X-Cost-Total-Micros')} microcents")
        print(f"  DynamoDB Reads: {response.headers.get('X-Cost-DynamoDB-Reads', '0')}")
        print(f"  DynamoDB Writes: {response.headers.get('X-Cost-DynamoDB-Writes', '0')}")


def test_actor_query() -> None:
    """Test the actor query."""
    query = """
    query GetActor($username: String) {
        actor(username: $username) {
            id
            username
            domain
            displayName
            summary
            followers
            following
            statusesCount
            bot
            locked
            createdAt
            updatedAt
        }
    }
    """

    variables = {"username": ACTOR_USERNAME}

    response = post_graphql({"query": query, "variables": variables})
    print("\n=== Actor Query ===")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")


def test_introspection() -> None:
    """Test GraphQL introspection to see available queries."""
    query = """
    query IntrospectionQuery {
        __schema {
            queryType {
                fields {
                    name
                    description
                    args {
                        name
                        type {
                            name
                            kind
                        }
                    }
                }
            }
        }
    }
    """

    response = post_graphql({"query": query})
    print("\n=== Schema Introspection ===")
    print(f"Status: {response.status_code}")

    if response.status_code == 200:
        data = response.json()
        schema = data.get("data", {}).get("__schema")
        if schema and schema.get("queryType"):
            print("\nAvailable Queries:")
            for field in schema["queryType"]["fields"]:
                args = ", ".join(
                    f"{arg['name']}: {arg['type']['name'] or arg['type']['kind']}"
                    for arg in field["args"]
                )
                print(f"  - {field['name']}({args})")


def test_graphql_error_handling() -> None:
    """Test GraphQL error handling with an invalid field."""
    query = """
    query InvalidQuery {
        nonExistentField
    }
    """

    response = post_graphql({"query": query})
    print("\n=== Error Handling Test ===")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")


if __name__ == "__main__":
    print(f"Testing GraphQL API at: {GRAPHQL_ENDPOINT} (stage={STAGE})")

    try:
        test_introspection()
        test_instance_metrics()
        test_actor_query()
        test_graphql_error_handling()
    except requests.exceptions.ConnectionError:
        print(f"\nError: Could not connect to GraphQL endpoint at {GRAPHQL_ENDPOINT}")
        print("Make sure the Lambda is running or deployed")
    except Exception as exc:  # pragma: no cover - diagnostic output
        print(f"\nUnexpected error: {exc}")
