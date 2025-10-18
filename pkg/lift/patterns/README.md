# Lift Event Patterns for Lesser

This package provides standardized patterns for handling various AWS Lambda event types with the Lift framework.

## Overview

The Lift framework primarily handles HTTP requests, but Lesser needs to process various AWS event types:
- DynamoDB Streams
- SQS Messages
- EventBridge/CloudWatch Events

This package provides reusable patterns and helpers to integrate these event types with Lift's middleware and error handling.

## Patterns

### DynamoDB Streams Pattern
Used by services that process DynamoDB change events (inserts, updates, deletes).

### SQS Pattern
Used by services that process messages from SQS queues.

### EventBridge Pattern
Used by services that run on schedules or respond to custom events.

## Usage

Each pattern provides:
1. Type-safe event extraction from Lift context
2. Proper error handling and logging
3. Middleware support
4. Batch processing utilities
5. Retry logic where appropriate