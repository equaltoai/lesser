# Accounts Service

The Accounts Service is part of Lesser's Phase 2.2 API Alignment implementation. It provides comprehensive account and profile management functionality with real-time streaming and ActivityPub federation support.

## Features

- **Profile Management**: Update display name, bio, avatar, header, and custom fields
- **Privacy Controls**: Account locking, discoverability, bot flags, and content sensitivity
- **User Preferences**: Timeline order, media expansion, language settings, and more
- **Privacy-Aware Queries**: Account retrieval with viewer-based privacy filtering
- **Account Search**: Search accounts with suspended account filtering
- **Real-Time Events**: Emits streaming events for profile updates and preferences changes
- **ActivityPub Federation**: Automatically queues Update activities for profile changes

## Core Operations

### UpdateProfile
Updates user profile information including display name, bio, images, and custom fields.

```go
cmd := &UpdateProfileCommand{
    Username:    "alice",
    DisplayName: "Alice Smith",
    Bio:         "Software developer and coffee enthusiast",
    Avatar:      "https://example.com/avatars/alice.jpg",
    Header:      "https://example.com/headers/alice-banner.jpg",
    Locked:      false,
    Bot:         false,
    Fields: []ProfileField{
        {Name: "Website", Value: "https://alice.dev"},
        {Name: "Location", Value: "San Francisco, CA"},
    },
    Discoverable: true,
    UpdaterID:    "alice", // Must match Username for authorization
}

result, err := service.UpdateProfile(ctx, cmd)
```

### UpdatePreferences
Updates user preferences for timeline behavior, media handling, and privacy settings.

```go
cmd := &UpdatePreferencesCommand{
    Username:                  "alice",
    Language:                  "en",
    DefaultPostingVisibility:  "public",
    DefaultMediaSensitive:     false,
    ExpandSpoilers:            true,
    ExpandMedia:               "default",
    AutoplayGifs:              true,
    ShowFollowCounts:          true,
    PreferredTimelineOrder:    "newest",
    SearchSuggestionsEnabled:  true,
    PersonalizedSearchEnabled: true,
    UpdaterID:                 "alice",
}

result, err := service.UpdatePreferences(ctx, cmd)
```

### GetAccount
Retrieves account information with privacy filtering based on the viewer.

```go
query := &GetAccountQuery{
    Username: "alice",
    ViewerID: "bob", // Optional - affects privacy filtering
}

account, err := service.GetAccount(ctx, query)
```

### SearchAccounts
Searches for accounts with filtering and privacy controls.

```go
query := &SearchAccountsQuery{
    Query:      "alice",
    ViewerID:   "bob",
    Pagination: interfaces.PaginationOptions{Limit: 20},
    Resolve:    false, // Don't resolve remote accounts
}

result, err := service.SearchAccounts(ctx, query)
```

## Event Emission

The service emits streaming events for real-time updates:

### Profile Updates
- **Event Type**: `account.updated`
- **Streams**: User's own stream (`user:username`) and followers stream (`followers:username`)
- **Federation**: Queues ActivityPub Update activity

### Preference Updates
- **Event Type**: `preferences.updated`
- **Streams**: User's own stream only (`user:username`)
- **Federation**: None (preferences are local only)

## Privacy and Security

### Authorization
- Profile updates require `UpdaterID` to match the account username
- Preference updates require `UpdaterID` to match the account username
- No admin override functionality (implement separately if needed)

### Privacy Filtering
- Suspended accounts are hidden from other users
- Email addresses are never exposed to other users
- Account information is sanitized based on viewer relationship

### Validation
- Display name limited to 100 characters
- Bio limited to 5,000 characters
- Maximum 4 custom profile fields
- Field names and values limited to 255 characters each
- Posting visibility must be valid (`public`, `unlisted`, `private`)

## Dependencies

- **AccountRepository**: For account data persistence
- **Publisher**: For real-time event streaming
- **FederationService**: For ActivityPub federation (optional)
- **Logger**: For structured logging

## Integration Example

```go
// Create the service
service := accounts.NewService(
    accountRepository,
    streamingPublisher,
    federationService,
    logger,
    "example.com", // domain name
)

// Update a user's profile
cmd := &accounts.UpdateProfileCommand{
    Username:    "alice",
    DisplayName: "Alice Smith",
    Bio:         "Updated bio",
    UpdaterID:   "alice",
}

result, err := service.UpdateProfile(ctx, cmd)
if err != nil {
    return err
}

// Result contains the updated account and emitted events
fmt.Printf("Updated account: %s\n", result.Account.User.DisplayName)
fmt.Printf("Events emitted: %d\n", len(result.Events))
```

## Testing

The service includes comprehensive tests covering:
- Successful operations
- Validation failures
- Authorization checks
- Privacy filtering
- Event emission
- Error handling

Run tests with:
```bash
go test ./pkg/services/accounts/... -v
```

## ActivityPub Compatibility

Profile updates are automatically federated using ActivityPub Update activities:
- Bot status is mapped to ActivityPub Actor Type (`Person` or `Service`)
- Profile fields are stored as `PropertyValue` attachments
- Profile images use standard ActivityPub `icon` and `image` properties
- Update activities are sent to followers' inboxes