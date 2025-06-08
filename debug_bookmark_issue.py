#!/usr/bin/env python3
import requests
import json
import time

# Configuration
BASE_URL = "https://lesser.host"
TOKEN = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhcm9uIiwiZXhwIjoxNzQ5MzQyOTc2LCJuYmYiOjE3NDkzMzkzNzYsImlhdCI6MTc0OTMzOTM3NiwidXNlcm5hbWUiOiJhcm9uIiwic2NvcGVzIjpbInJlYWQiLCJ3cml0ZSIsImZvbGxvdyIsInB1c2giLCJhZG1pbiJdLCJjbGllbnRfaWQiOiJEUlF5bzhSWGNvbGExWEpCYlZ2Um9BPT0ifQ.cHRpDDk_I1PGwzK_9TrB6K1aKYw6S51xl8NKbvB0K24"

headers = {
    "Authorization": f"Bearer {TOKEN}",
    "Content-Type": "application/json"
}

print("1. Creating a test status...")
response = requests.post(
    f"{BASE_URL}/api/v1/statuses",
    headers=headers,
    json={
        "status": "Test status for bookmark debugging",
        "visibility": "public"
    }
)

if response.status_code not in [200, 201]:
    print(f"Failed to create status: {response.status_code}")
    print(response.text)
    exit(1)

status = response.json()
status_id_full = status['id']
print(f"Created status with ID: {status_id_full}")

# Extract different ID formats
if status_id_full.startswith('http'):
    status_id_short = status_id_full.split('/')[-1]
else:
    status_id_short = status_id_full

print(f"Short ID: {status_id_short}")
print(f"Full ID: {status_id_full}")

# Wait a moment for consistency
print("\n2. Waiting 1 second for consistency...")
time.sleep(1)

# Try to get the status with short ID
print(f"\n3. Testing GET /api/v1/statuses/{status_id_short}")
response = requests.get(
    f"{BASE_URL}/api/v1/statuses/{status_id_short}",
    headers=headers
)
print(f"Response: {response.status_code}")
if response.status_code == 200:
    print("✅ Status retrieved successfully with short ID")
else:
    print(f"❌ Failed: {response.text}")

# Try to bookmark with short ID
print(f"\n4. Testing POST /api/v1/statuses/{status_id_short}/bookmark")
response = requests.post(
    f"{BASE_URL}/api/v1/statuses/{status_id_short}/bookmark",
    headers=headers
)
print(f"Response: {response.status_code}")
if response.status_code == 200:
    print("✅ Bookmark successful with short ID")
else:
    print(f"❌ Failed: {response.text}")

# Try to bookmark with full URL (if different)
if status_id_full != status_id_short:
    print(f"\n5. Testing POST /api/v1/statuses/{status_id_full}/bookmark")
    response = requests.post(
        f"{BASE_URL}/api/v1/statuses/{status_id_full}/bookmark",
        headers=headers
    )
    print(f"Response: {response.status_code}")
    if response.status_code == 200:
        print("✅ Bookmark successful with full ID")
    else:
        print(f"❌ Failed: {response.text}")

# Clean up
print(f"\n6. Cleaning up - deleting status {status_id_short}")
response = requests.delete(
    f"{BASE_URL}/api/v1/statuses/{status_id_short}",
    headers=headers
)
print(f"Delete response: {response.status_code}") 