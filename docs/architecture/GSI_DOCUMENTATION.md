# Global Secondary Index (GSI) Documentation

## Overview

Lesser uses DynamoDB Global Secondary Indexes (GSIs) to enable efficient queries across different access patterns. This document details the purpose and usage of each GSI.

## GSI Inventory

| GSI | Primary Purpose | Key Access Patterns |
|-----|----------------|-------------------|
| GSI1 | Multi-purpose user and entity queries | User activities, inbox, quotes, relationships, search |
| GSI2 | Status, visibility, and moderation queries | Community notes, name search, reports, flags |
| GSI3 | Author, content, and categorization | Author content, hashtags, likes, moderation events |
| GSI4 | Actor metrics and temporal queries | Actor rankings, announces, active users |
| GSI5 | Blocks and activity tracking | Block relationships, daily active users |
| GSI6 | Reply threading | Direct reply relationships |

## Detailed GSI Usage

### GSI1 - Multi-Purpose User and Entity Index

GSI1 is our most versatile index, supporting numerous access patterns:

#### Access Patterns:
1. **User Activities & Jobs**
   - GSI1PK: `USER#<username>`
   - GSI1SK: `CREATED#<timestamp>` or `REPORT#<timestamp>`
   - Used for: Import/export jobs, user reports

2. **Inbox Activities**
   - GSI1PK: `INBOX#<username>`
   - GSI1SK: `<timestamp>`
   - Used for: User inbox queries

3. **Refresh Token Lookup**
   - GSI1PK: `REFRESHTOKEN#<token>`
   - GSI1SK: `<session-id>`
   - Used for: OAuth session management

4. **Quote Relationships**
   - GSI1PK: `QUOTE_TARGET#<target-note-id>`
   - GSI1SK: `TIMESTAMP#<unix-timestamp>`
   - Used for: Finding all quotes of a specific note

5. **Relationship Queries**
   - GSI1PK: `FOLLOW#<username>` or `FOLLOWING#<username>`
   - GSI1SK: `FOLLOWER#<username>` or `<timestamp>`
   - Used for: Follow/follower relationships

6. **Username Search**
   - GSI1PK: `USERNAME_SEARCH#<first-2-chars>`
   - GSI1SK: `<username>`
   - Used for: Prefix-based username search

7. **Moderation Decisions**
   - GSI1PK: `ACTIVE_DECISIONS` or `ACTOR#<actor-id>`
   - GSI1SK: `OBJECT#<object-id>` or `TIME#<timestamp>`
   - Used for: Active moderation decisions

8. **Domain Management**
   - GSI1PK: `DOMAIN_BLOCKS`, `DOMAIN_ALLOWS`, `EMAIL_DOMAIN_BLOCKS`
   - GSI1SK: `<timestamp>`
   - Used for: Federation allow/block lists

9. **Collection Items**
   - GSI1PK: `ITEM#<item-id>`
   - GSI1SK: `COLLECTION#<collection-name>`
   - Used for: Reverse collection lookups

10. **Conversations**
    - GSI1PK: `CONVERSATION#<conversation-id>`
    - GSI1SK: `PARTICIPANT#<participant-id>`
    - Used for: Conversation participants

11. **Push Subscriptions**
    - GSI1PK: `PUSH_ENDPOINT#<endpoint-hash>`
    - GSI1SK: `<username>`
    - Used for: Endpoint deduplication

12. **Scheduled Statuses**
    - GSI1PK: `SCHEDULED#DUE`
    - GSI1SK: `TIME#<scheduled-time>#ID#<id>`
    - Used for: Due scheduled posts

13. **Object Notes**
    - GSI1PK: `OBJECT#<object-id>#NOTES`
    - GSI1SK: `SCORE#<score>#<note-id>`
    - Used for: Community notes by score

14. **Move Activities**
    - GSI1PK: `MOVE#TARGET#<target>`
    - GSI1SK: `ACTOR#<actor>`
    - Used for: Account migrations

15. **Mutes**
    - GSI1PK: `MUTED#<muted-user>`
    - GSI1SK: `MUTER#<muting-user>`
    - Used for: Reverse mute lookups

16. **Hashtag Followers**
    - GSI1PK: `HASHTAG#<tag>`
    - GSI1SK: `USER#<username>`
    - Used for: Hashtag followers

17. **Active Federation**
    - GSI1PK: `FEDERATION_ACTIVE`
    - GSI1SK: `<last-seen-timestamp>#<domain>`
    - Used for: Active federated instances

21. **Federation Graph Nodes**
    - GSI1PK: `FEDERATION_GRAPH#NODES`
    - GSI1SK: `<health>#<domain>`
    - Used for: Federation node discovery by health status

18. **Actor Objects**
    - GSI1PK: `ACTOR#<username>#OBJECTS`
    - GSI1SK: `<published-timestamp>`
    - Used for: Actor's content timeline

19. **Status Polls**
    - GSI1PK: `STATUS#<status-id>`
    - GSI1SK: `POLL`
    - Used for: Poll lookups by status

20. **Recovery Tokens**
    - GSI1PK: `RECOVERY#TOKEN`
    - GSI1SK: `<token>#<timestamp>`
    - Used for: Token validation

### GSI2 - Status, Visibility, and Moderation Queries

#### Access Patterns:
1. **Display Name Search**
   - GSI2PK: `NAME_SEARCH#<first-2-chars>`
   - GSI2SK: `<display-name>#<username>`
   - Used for: Display name prefix search

2. **Community Notes by Status**
   - GSI2PK: `NOTES#<visibility-status>`
   - GSI2SK: `<timestamp>#<note-id>`
   - Used for: Note moderation queues

3. **Reports by Target**
   - GSI2PK: `REPORTED#<target-account-id>`
   - GSI2SK: `REPORT#<timestamp>`
   - Used for: Reports against an account

4. **Moderation Events by Type**
   - GSI2PK: `TYPE#<event-type>#<category>`
   - GSI2SK: `SEVERITY#<severity>#<timestamp>`
   - Used for: Categorized moderation events

5. **Flags by Object**
   - GSI2PK: `FLAG#OBJECT#<object-id>`
   - GSI2SK: `CREATED#<timestamp>`
   - Used for: Flags on specific content

6. **Vouches Received**
   - GSI2PK: `VOUCHEE#<to-user>`
   - GSI2SK: `FROM#<from-user>`
   - Used for: Trust/reputation system

7. **Instance Connections**
   - GSI2PK: `INSTANCE#<domain>#CONNECTIONS#<type>`
   - GSI2SK: `<timestamp>#<target-domain>`
   - Used for: Instance connection queries by type

8. **Federation Edges by Volume**
   - GSI2PK: `FEDERATION_EDGES#<connection-type>`
   - GSI2SK: `VOLUME#<padded-volume>#<source>#<target>`
   - Used for: Finding strongest connections by type

### GSI3 - Author, Content, and Categorization

#### Access Patterns:
1. **Domain Members**
   - GSI3PK: `DOMAIN#<domain>`
   - GSI3SK: `<username>`
   - Used for: Users from specific domains

2. **Author's Community Notes**
   - GSI3PK: `AUTHOR#<author-id>#NOTES`
   - GSI3SK: `<timestamp>#<note-id>`
   - Used for: Notes by author

3. **Actor's Likes**
   - GSI3PK: `ACTOR#<actor-id>#LIKES`
   - GSI3SK: `PUBLISHED#<timestamp>#OBJECT#<object-id>`
   - Used for: Like history

4. **Reports by Status**
   - GSI3PK: `STATUS#<status>`
   - GSI3SK: `REPORT#<timestamp>`
   - Used for: Report filtering

5. **Moderation Event Lookup**
   - GSI3PK: `EVENTID#<event-id>`
   - GSI3SK: `EVENTID#<event-id>`
   - Used for: Direct event access

6. **Hashtag Search**
   - GSI3PK: `HASHTAG_SEARCH#<prefix>`
   - GSI3SK: `<hashtag>`
   - Used for: Hashtag autocomplete

7. **Featured Tags**
   - GSI3PK: `ACTOR#<actor>#FEATURED_TAGS`
   - GSI3SK: `TAG#<tag>`
   - Used for: Actor's featured tags

8. **Trust Metrics**
   - GSI3PK: `TRUST#NOTABLE`
   - GSI3SK: `SCORE#<score>#ACTOR#<actor>`
   - Used for: High-trust actors

9. **Federation Domain Metadata**
   - GSI3PK: `DOMAIN#<domain>`
   - GSI3SK: `FEDERATION_NODE`
   - Used for: Domain federation lookups

10. **Instance Clusters**
    - GSI3PK: `FEDERATION_CLUSTER#<cluster-id>`
    - GSI3SK: `MEMBER#<domain>`
    - Used for: Cluster membership queries

### GSI4 - Actor Metrics and Temporal Queries

#### Access Patterns:
1. **Actor Rankings by Followers**
   - GSI4PK: `ACTOR_RANK#<bucket>` (e.g., 1-100, 100-1k, 1k-10k)
   - GSI4SK: Formatted follower count with username
   - Used for: Popular account discovery

2. **Actor's Announces**
   - GSI4PK: `ACTOR#<actor-id>#ANNOUNCES`
   - GSI4SK: `PUBLISHED#<timestamp>#OBJECT#<object-id>`
   - Used for: Announce/boost history

3. **AI Analysis Queue**
   - GSI4PK: `AI_ANALYSIS#<date>`
   - GSI4SK: `<timestamp>`
   - Used for: AI processing pipeline

### GSI5 - Blocks and Activity Tracking

#### Access Patterns:
1. **Block Relationships**
   - GSI5PK: `BLOCKED#<blocked-user>`
   - GSI5SK: `BLOCKER#<blocking-user>`
   - Used for: Reverse block lookups

2. **Daily Active Users**
   - GSI5PK: `ACTIVE#<date>`
   - GSI5SK: `<timestamp>#<username>`
   - Used for: Activity metrics

### GSI6 - Reply Threading

#### Access Patterns:
1. **Direct Replies**
   - GSI6PK: `REPLIES#<parent-object-url>`
   - GSI6SK: `<timestamp>#<reply-object-url>`
   - Used for: Efficient reply retrieval for context endpoints

## Best Practices

1. **Reuse Before Creating**: Always check if an existing GSI can serve your access pattern
2. **Document Usage**: Update this document when adding new access patterns
3. **Consider Hot Partitions**: Design partition keys to distribute load evenly
4. **Timestamp Sorting**: Use timestamps in sort keys for chronological ordering
5. **Prefix Patterns**: Use clear prefixes (USER#, ACTOR#, etc.) to avoid collisions

## Adding New Access Patterns

Before creating a new GSI:

1. Check if any existing GSI can accommodate your pattern
2. Consider the cardinality of your partition key
3. Ensure even distribution of data across partitions
4. Document the pattern in this file
5. Add integration tests for the new query pattern

## Cost Considerations

- Each GSI incurs storage costs (duplicates item attributes)
- Query costs are based on consumed read capacity units
- Sparse indexes (not all items have GSI attributes) are more cost-effective
- Monitor GSI usage through CloudWatch metrics

## Migration Notes

When adding GSI attributes to existing items:
- New items automatically include GSI attributes
- Existing items need migration or will be missing from GSI queries
- Consider lazy migration (update on next write) vs batch migration

---

*Last updated: 2025-06-21*