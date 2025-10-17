# DynamoDB GSI Usage and Patterns Guide

This document provides a comprehensive overview of how Global Secondary Indexes (GSIs) are used in the `lesser` application. It covers existing patterns, provides guidance for implementing new query patterns, and offers recommendations for maintaining a scalable and cost-effective data model.

## 1. Overview of DynamoDB Strategy

The application employs a **single-table design** in DynamoDB, with a main table named `lesser-{environment}`. This design is highly scalable and cost-effective, but it relies heavily on GSIs to support various query patterns beyond simple key-value lookups.

The primary key for the table is composed of:
- **Partition Key (PK):** `PK`
- **Sort Key (SK):** `SK`

To support complex query requirements, the infrastructure pre-provisions **9 generic GSIs (`GSI1` through `GSI9`)**. Each GSI has its own key schema:
- **Partition Key:** `GSI<N>PK`
- **Sort Key:** `GSI<N>SK`

These generic GSIs are "overloaded" by different data models to serve a wide variety of access patterns.

## 2. Existing GSI Patterns and Implementations

Here are some examples of how GSIs are currently used in key data models.

### a. `RelationshipRecord` (`pkg/storage/models/relationship.go`)

This model uses three GSIs to manage user relationships (follows).

-   **GSI1: Inverted Index for Reverse Lookups**
    -   **Purpose:** To efficiently find all followers of a given user. The primary key allows finding who a user *is following*, but not who *is following them*.
    -   **PK/SK:** `FOLLOW#{followerUsername}` / `FOLLOWING#{followingUsername}`
    -   **GSI1PK/GSI1SK:** `FOLLOW#{followedUsername}` / `FOLLOWER#{followerUsername}`
    -   **Pattern:** This classic **inverted index** pattern allows for a fast reverse lookup without scanning the entire table.

-   **GSI2 & GSI3: Domain-Specific Queries**
    -   **Purpose:** To query relationships by domain, which is critical for federation and detecting server-wide "severance" events.
    -   **GSI2PK:** `FOLLOWER_DOMAIN#{domain}` (Finds all remote users from a specific domain that follow local users)
    -   **GSI3PK:** `FOLLOWING_DOMAIN#{domain}` (Finds all remote users that local users are following on a specific domain)
    -   **Pattern:** This demonstrates using GSIs to create secondary indexes on specific attributes (in this case, a derived one like the domain).

### b. `ModerationSample` (`pkg/storage/models/moderation_ml.go`)

This model uses GSIs to support various query patterns for the machine learning moderation system.

-   **GSI1: Query by Secondary Attribute**
    -   **Purpose:** To find all moderation samples submitted by a specific reviewer.
    -   **GSI1PK:** `REVIEWER#{reviewer_id}`

-   **GSI2: Query by Category and Score**
    -   **Purpose:** To find samples with a specific label, sorted by confidence score.
    -   **GSI2PK:** `LABEL#{label}`
    -   **GSI2SK:** `CONFIDENCE#{confidence}#{RFC3339}`

-   **GSI3: Direct ID Lookup**
    -   **Purpose:** To allow a direct lookup of a moderation sample by its unique ID, which is not part of the primary key.
    -   **GSI3PK/GSI3SK:** `SAMPLEID#{sample_id}` / `SAMPLEID#{sample_id}`

### c. `TranscodingJob` (`pkg/storage/models/transcoding_job.go`)

This model uses GSIs to track media transcoding jobs.

-   **GSI1 & GSI2: Querying by Associated Entities**
    -   **Purpose:** To find all transcoding jobs associated with a specific user or a specific media item.
    -   **GSI1PK:** `USER_TRANSCODING#{userID}`
    -   **GSI2PK:** `MEDIA_TRANSCODING#{mediaID}`

**Note on Naming Consistency:** The `TranscodingJob` model uses descriptive names in its struct tags (e.g., `index:user-jobs-index`). The `dynamorm` library maps this to an available GSI. While functionally correct, this is an inconsistency with other models that directly reference `GSI1`, etc. For clarity, it is recommended to standardize on one approach.

## 3. Guide to Adding a New Query Pattern

When you need to query data using attributes that are not in the primary key, follow these steps to implement a new GSI-backed query pattern.

### Step 1: Identify the Query Pattern

Clearly define the access pattern you need. For example: "I need to find all users with a specific role" or "I need to find all statuses that mention a specific hashtag, sorted by date."

### Step 2: Choose an Available GSI

Consult the central GSI documentation (see recommendations below) to find an unused GSI or an existing GSI that can be overloaded, ensuring there are no key collisions.

### Step 3: Update the Go Model

In the relevant model struct within `pkg/storage/models/`, add the fields for the GSI partition and sort keys. Use `dynamorm` struct tags to associate them with the chosen GSI.

```go
// Example for finding users by role using GSI4
type User struct {
    // ... existing fields ...
    PK     string `dynamorm:"pk"` // USER#{username}
    SK     string `dynamorm:"sk"` // METADATA

    // GSI4 for role-based queries
    GSI4PK string `dynamorm:"index:gsi4,pk" json:"gsi4pk,omitempty"` // ROLE#{role}
    GSI4SK string `dynamorm:"index:gsi4,sk" json:"gsi4sk,omitempty"` // USER#{username}

    Role   string `json:"role"`
    // ... other fields ...
}
```

### Step 4: Populate the GSI Keys

In the same model file, implement or update a hook like `BeforeCreate()` or `BeforeUpdate()` to populate the new GSI key fields. The logic should derive the key values from other fields in the model.

```go
func (u *User) BeforeUpdate() error {
    // ... other update logic ...

    // Populate GSI4 keys
    if u.Role != "" {
        u.GSI4PK = fmt.Sprintf("ROLE#%s", u.Role)
        u.GSI4SK = fmt.Sprintf("USER#%s", u.Username)
    } else {
        // Ensure keys are empty if the attribute is not set, creating a sparse GSI
        u.GSI4PK = ""
        u.GSI4SK = ""
    }

    return nil
}
```

### Step 5: Implement the Repository Method

Add a new method to the corresponding repository in `pkg/storage/repositories/` that uses the GSI to perform the query.

## 4. Maintenance and Best Practices

### a. Create a Central GSI Registry

The most critical step for maintaining a single-table design is to have a centralized document that tracks GSI usage. This prevents key collisions and makes it easy to find available GSIs.

**Recommendation:** Create a file, such as `docs/architecture/dynamodb_gsi_registry.md`, and document each GSI (`GSI1` through `GSI9`) with the following information:
- **GSI Name:** e.g., `GSI1`
- **Purpose:** A brief description (e.g., "Relationship reverse lookups, Moderation reviewer queries").
- **Used By Models:** List of Go models that use this GSI.
- **Key Schema Patterns:** Document the `PK` and `SK` patterns for each model using the GSI (e.g., `RelationshipRecord: FOLLOW#{followedUsername} / FOLLOWER#{followerUsername}`).

This registry should be the single source of truth for GSI usage and should be updated whenever a new pattern is added.

### b. Monitor Cost and Performance

GSIs add to the cost of writes and have their own provisioned throughput.

**Recommendation:**
- Set up **CloudWatch Alarms** for the consumed read and write capacity of each GSI. This will help you identify "hot" GSIs that may be causing throttling or high costs.
- Regularly review your DynamoDB costs in the AWS Billing console to understand the cost impact of your GSIs.

### c. Use Sparse GSIs

A GSI is "sparse" if its key attributes are only present on a subset of items in the table. This is a powerful and cost-effective pattern.

**Recommendation:**
- When designing a new query pattern, consider if it only applies to a small fraction of your data (e.g., "is_pending", "has_error").
- To create a sparse GSI, simply leave the GSI key fields empty in your model's `BeforeCreate`/`BeforeUpdate` hooks for items that should not be indexed. This saves on both storage and write costs.

### d. Maintain Naming Consistency

Ensure that key prefixes (e.g., `USER#`, `ROLE#`) are consistent and documented. Avoid using the same prefix for different types of data to prevent collisions, especially when overloading GSIs.
