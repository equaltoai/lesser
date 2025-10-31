#!/usr/bin/env python3
"""
Check timeline data diagnostic
"""
import os
import json
import requests

GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", "https://dev.lesser.host/api/graphql")
TOKEN = os.getenv("GRAPHQL_TOKEN")

def graphql_request(query, variables=None):
    """Execute a GraphQL request"""
    headers = {
        "Authorization": TOKEN if TOKEN.startswith("Bearer ") else f"Bearer {TOKEN}",
        "Content-Type": "application/json"
    }
    
    payload = {"query": query}
    if variables:
        payload["variables"] = variables
    
    response = requests.post(GRAPHQL_ENDPOINT, json=payload, headers=headers, timeout=30)
    return response.json()

def check_actor():
    """Check actor to see status count"""
    query = """
    query GetActor {
        actor(username: "admin") {
            id
            username
            statusesCount
        }
    }
    """
    result = graphql_request(query)
    print("=== Actor Check ===")
    print(json.dumps(result, indent=2))

def check_home_timeline():
    """Check home timeline"""
    query = """
    query HomeTimeline {
        timeline(type: HOME, first: 10) {
            edges {
                node {
                    id
                    content
                    visibility
                }
            }
        }
    }
    """
    result = graphql_request(query)
    print("\n=== Home Timeline ===")
    print(json.dumps(result, indent=2))

def check_public_timeline():
    """Check public timeline"""
    query = """
    query PublicTimeline {
        timeline(type: PUBLIC, first: 10) {
            edges {
                node {
                    id
                    content
                    visibility
                }
            }
        }
    }
    """
    result = graphql_request(query)
    print("\n=== Public Timeline ===")
    print(json.dumps(result, indent=2))

def check_local_timeline():
    """Check local timeline"""
    query = """
    query LocalTimeline {
        timeline(type: LOCAL, first: 10) {
            edges {
                node {
                    id
                    content
                    visibility
                }
            }
        }
    }
    """
    result = graphql_request(query)
    print("\n=== Local Timeline ===")
    print(json.dumps(result, indent=2))

if __name__ == "__main__":
    check_actor()
    check_home_timeline()
    check_public_timeline()
    check_local_timeline()

