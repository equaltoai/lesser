#!/usr/bin/env python3
"""
Clear all data from DynamoDB table (except system/bootstrapped users if needed)
This script scans the entire table and deletes all items.
"""
import os
import sys
import time
import boto3
from botocore.exceptions import ClientError

# Get table name from environment or default
TABLE_NAME = os.environ.get("DYNAMODB_TABLE", "lesser-development")
AWS_PROFILE = os.environ.get("AWS_PROFILE", "Lesser")
AWS_REGION = os.environ.get("AWS_REGION", "us-east-1")

# Optional: Preserve bootstrap users (admin, member, mod)
PRESERVE_USERS = os.environ.get("PRESERVE_USERS", "false").lower() == "true"
PRESERVED_USERNAMES = {"admin", "member", "mod"}


def get_dynamodb_client():
    """Create DynamoDB client with profile"""
    session = boto3.Session(profile_name=AWS_PROFILE)
    return session.client("dynamodb", region_name=AWS_REGION)


def clear_table(client, table_name):
    """Clear all items from DynamoDB table"""
    print(f"Clearing table: {table_name}")
    print(f"AWS Profile: {AWS_PROFILE}")
    print(f"Region: {AWS_REGION}")
    
    if PRESERVE_USERS:
        print("Preserving bootstrap users: admin, member, mod")
    
    deleted_count = 0
    preserved_count = 0
    
    # Use paginator to handle large tables
    paginator = client.get_paginator("scan")
    
    for page in paginator.paginate(TableName=table_name):
        items = page.get("Items", [])
        
        if not items:
            continue
        
        # Batch delete items (DynamoDB batch_write limit is 25 items)
        batch_size = 25
        for i in range(0, len(items), batch_size):
            batch = items[i : i + batch_size]
            
            # Prepare delete requests
            delete_requests = []
            for item in batch:
                pk = item.get("PK", {}).get("S", "")
                sk = item.get("SK", {}).get("S", "")
                
                # Check if we should preserve this user
                if PRESERVE_USERS:
                    # Check if PK or SK contains preserved usernames
                    should_preserve = False
                    for username in PRESERVED_USERNAMES:
                        if username in pk.lower() or username in sk.lower():
                            # Check if it's a USER record (not STATUS, etc.)
                            if "USER#" in pk.upper() or pk.startswith(f"USER#{username.upper()}"):
                                should_preserve = True
                                break
                    
                    if should_preserve:
                        preserved_count += 1
                        continue
                
                # Add to delete batch
                delete_requests.append(
                    {
                        "DeleteRequest": {
                            "Key": {
                                "PK": item["PK"],
                                "SK": item["SK"],
                            }
                        }
                    }
                )
            
            # Execute batch delete
            if delete_requests:
                try:
                    response = client.batch_write_item(
                        RequestItems={table_name: delete_requests}
                    )
                    
                    # Handle unprocessed items (retry logic)
                    unprocessed = response.get("UnprocessedItems", {})
                    retry_count = 0
                    max_retries = 5
                    
                    while unprocessed and retry_count < max_retries:
                        retry_count += 1
                        time.sleep(0.5 * retry_count)  # Exponential backoff
                        response = client.batch_write_item(RequestItems=unprocessed)
                        unprocessed = response.get("UnprocessedItems", {})
                    
                    deleted_count += len(delete_requests)
                    print(f"  Deleted {deleted_count} items...", end="\r")
                    
                except ClientError as e:
                    print(f"\nError deleting batch: {e}")
                    sys.exit(1)
    
    print(f"\n✓ Cleared {deleted_count} items from table")
    if PRESERVE_USERS:
        print(f"✓ Preserved {preserved_count} user records")
    
    return deleted_count


def main():
    """Main entry point"""
    try:
        client = get_dynamodb_client()
        
        # Verify table exists
        try:
            client.describe_table(TableName=TABLE_NAME)
        except ClientError as e:
            if e.response["Error"]["Code"] == "ResourceNotFoundException":
                print(f"Error: Table {TABLE_NAME} does not exist")
                sys.exit(1)
            raise
        
        # Clear the table
        deleted_count = clear_table(client, TABLE_NAME)
        
        if deleted_count == 0:
            print("Table is already empty")
        else:
            print(f"Successfully cleared {deleted_count} items")
        
        sys.exit(0)
        
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()

