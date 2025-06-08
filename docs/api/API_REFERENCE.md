# Lesser API Reference

## Overview

Lesser provides a **100% complete** Mastodon-compatible REST API plus a modern GraphQL API. All endpoints are implemented and production-ready.

### API Status
- ✅ **REST API**: Full Mastodon v1 compatibility achieved
- ✅ **GraphQL API**: All 60 operations implemented
- ✅ **Streaming API**: WebSocket support for real-time updates
- ✅ **OAuth 2.0**: Complete implementation with PKCE
- ✅ **Federation**: Full ActivityPub protocol support

### Base URL
```
https://your-instance.example.com
```

### Authentication
Most endpoints require authentication via OAuth 2.0 bearer tokens:
```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

### Rate Limiting
- Authenticated requests: 300 per 5 minutes
- Unauthenticated requests: 100 per 5 minutes
- Headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`

### Cost Tracking
Every response includes cost tracking headers:
```
X-Cost-Total-Micros: 125
X-Cost-DynamoDB-Reads: 1
X-Cost-DynamoDB-Writes: 0
```

**Official Mastodon API Documentation**: https://docs.joinmastodon.org/api/

## Core Concepts

### 1. Every Response Includes Cost Data
```json
{
  "data": { ... },
  "cost": {
    "total_cost_micros": 234,  // $0.000234
    "breakdown": { ... }
  },
  "duration_ms": 12
}
```

### 2. Real-Time Streams Available
- **WebSocket**: `wss://instance.com/api/v1/streaming`
- **Server-Sent Events**: `https://instance.com/api/v1/streaming/events`

### 3. GraphQL Alternative
- **Endpoint**: `https://instance.com/api/graphql`
- **Subscriptions**: Real-time updates via WebSocket

## Part 1: Mastodon Client Interface (Primary UI)

This is the main social media interface that users interact with daily. It should feel familiar to Mastodon users while subtly incorporating Lesser's innovations.

### 1.1 User Profile System

#### Profile Display
**Endpoint**: `GET /api/v1/accounts/:id`
**Docs**: https://docs.joinmastodon.org/methods/accounts/#get

**Complete Profile Structure**:
```typescript
interface UserProfile {
  // Core Identity
  id: string;
  username: string;              // Handle without domain
  acct: string;                  // Full handle (user@domain)
  display_name: string;          // Chosen display name
  locked: boolean;               // Requires approval to follow
  bot: boolean;                  // Automated account flag
  group: boolean;                // Group account flag
  discoverable: boolean;         // Appears in directory
  
  // Profile Content
  note: string;                  // Bio (HTML)
  avatar: string;                // Avatar URL
  avatar_static: string;         // Static avatar URL
  header: string;                // Header image URL
  header_static: string;         // Static header URL
  
  // Metadata
  fields: ProfileField[];        // Up to 4 custom fields
  emojis: CustomEmoji[];         // Custom emojis in bio
  
  // Statistics
  created_at: string;
  last_status_at: string;
  statuses_count: number;
  followers_count: number;
  following_count: number;
  
  // Lesser Enhancements
  trust_indicators: {
    score: number;               // 0-100 overall trust
    badges: TrustBadge[];        // Visual trust indicators
    verification_level: 'none' | 'email' | 'phone' | 'government';
  };
  
  cost_transparency: {
    monthly_cost: number;        // User's instance costs
    cost_per_post: number;       // Average post cost
    storage_used: number;        // MB of media stored
  };
}

interface ProfileField {
  name: string;                  // Field label
  value: string;                 // Field content (HTML)
  verified_at?: string;          // Verification timestamp
  
  // Lesser Enhancement
  trust_verified?: boolean;      // Community verification
}
```

**UI Design Requirements**:

1. **Header Section** (320px height on desktop, 160px mobile):
   - Header image with gradient overlay for text readability
   - Avatar (90px desktop, 70px mobile) with trust badge overlay
   - Display name in bold (24px desktop, 20px mobile)
   - @handle in muted color with instance health dot
   - Lock icon if account is locked
   - Bot/Group badges if applicable

2. **Bio Section**:
   - Render HTML safely (support `<p>`, `<br>`, `<a>`, `<span>`)
   - Custom emoji support with 20px height
   - "Show more" for bios over 500 characters
   - Translate button for non-native languages

3. **Profile Fields** (max 4):
   - Icon for verified fields (green checkmark)
   - Community verification indicator for Lesser
   - Links should show favicon
   - Truncate long values with ellipsis

4. **Statistics Bar**:
   - Large numbers should use K/M notation (e.g., 1.2K)
   - Clickable to view followers/following lists
   - Show "mutual" indicator for mutual follows
   - Cost indicator as subtle addition

5. **Action Buttons**:
   - Follow/Unfollow (primary color when not following)
   - Bell icon for notifications (on/off state)
   - More menu (mention, share, block, report)
   - Trust score visible on hover

#### Profile Editing
**Endpoint**: `PATCH /api/v1/accounts/update_credentials`
**Docs**: https://docs.joinmastodon.org/methods/accounts/#update_credentials

**Edit Form Requirements**:
```typescript
interface ProfileEditForm {
  // Basic Info
  display_name: string;          // Max 30 chars
  note: string;                  // Max 500 chars
  avatar: File;                  // Max 2MB, square aspect
  header: File;                  // Max 2MB, 3:1 aspect
  
  // Privacy Settings
  locked: boolean;               // Require follow approval
  bot: boolean;                  // This is a bot account
  discoverable: boolean;         // List in directory
  
  // Profile Fields
  fields_attributes: {
    name: string;                // Max 255 chars
    value: string;               // Max 255 chars
  }[];                          // Max 4 fields
  
  // Lesser Additions
  trust_preferences: {
    allow_vouching: boolean;
    show_cost_data: boolean;
    participate_in_moderation: boolean;
  };
}
```

**UI Components**:
- **Image Croppers**: Square for avatar, 3:1 for header
- **Character Counters**: Live updating, red when over limit
- **Field Manager**: Add/remove/reorder fields with drag handle
- **Preview Mode**: Show how profile will appear
- **Cost Preview**: Show storage cost of uploaded media

### 1.2 Lists Management

**Docs**: https://docs.joinmastodon.org/methods/lists/

#### List Overview
**Endpoint**: `GET /api/v1/lists`

```typescript
interface List {
  id: string;
  title: string;
  replies_policy: 'followed' | 'list' | 'none';
  exclusive: boolean;            // Hide list members from home
  
  // Lesser Enhancements
  member_count: number;
  estimated_cost: number;        // Cost of list timeline
  last_active: string;          // Last post in list
}
```

**List Management UI**:
1. **List Grid/List View Toggle**
2. **Create New List** button (prominent)
3. **List Cards** showing:
   - Title with member count
   - Last activity time
   - Quick add/remove members
   - Settings gear icon
4. **Sorting Options**: Name, activity, member count
5. **Search Lists** functionality

#### List Timeline
**Endpoint**: `GET /api/v1/timelines/list/:list_id`

**Design Requirements**:
- Same as home timeline but with list header
- Show list title and member count at top
- Quick access to list settings
- "Add members" call-to-action if empty

#### Managing List Members
**Endpoints**:
- `GET /api/v1/lists/:id/accounts` - Get members
- `POST /api/v1/lists/:id/accounts` - Add members
- `DELETE /api/v1/lists/:id/accounts` - Remove members

**Member Management UI**:
```typescript
interface ListMemberManager {
  search: string;                // Search followed accounts
  selected: string[];            // Multi-select support
  
  // Bulk operations
  bulk_add: boolean;
  bulk_remove: boolean;
  
  // Lesser Enhancement
  cost_impact: {
    current: number;
    after_changes: number;
    difference: number;
  };
}
```

**Design Elements**:
- **Two-column layout**: Available accounts | Current members
- **Drag and drop** between columns
- **Search bars** for each column
- **Bulk selection** with checkboxes
- **Cost indicator** showing impact of changes

### 1.3 Timeline Views (Detailed)

#### Home Timeline
**Endpoint**: `GET /api/v1/timelines/home`
**Docs**: https://docs.joinmastodon.org/methods/timelines/#home

**Complete Timeline Structure**:
```typescript
interface TimelinePost {
  // Core Status Data
  id: string;
  created_at: string;
  in_reply_to_id?: string;
  in_reply_to_account_id?: string;
  sensitive: boolean;
  spoiler_text: string;
  visibility: 'public' | 'unlisted' | 'private' | 'direct';
  language: string;
  uri: string;
  url?: string;
  replies_count: number;
  reblogs_count: number;
  favourites_count: number;
  edited_at?: string;
  
  // Content
  content: string;               // HTML content
  text?: string;                 // Plain text for API
  
  // Relationships
  favourited?: boolean;
  reblogged?: boolean;
  muted?: boolean;
  bookmarked?: boolean;
  pinned?: boolean;
  
  // Rich Media
  media_attachments: MediaAttachment[];
  mentions: Mention[];
  tags: Tag[];
  emojis: CustomEmoji[];
  card?: PreviewCard;
  poll?: Poll;
  
  // Account
  account: Account;
  
  // Reblog
  reblog?: Status;               // If this is a boost
  
  // Lesser Enhancements
  delivery_cost: number;
  trust_context: {
    author_score: number;
    content_safety: number;
    community_notes?: Note[];
    moderation_flags?: Flag[];
  };
}
```

**Timeline UI Components**:

1. **Post Card Structure**:
   ```
   ┌─────────────────────────────────────┐
   │ [Avatar] Name @handle · 5m    [...] │ <- Header
   │ [Trust] [Cost]              [Pin]   │
   ├─────────────────────────────────────┤
   │ Post content goes here...           │ <- Content
   │ Show more                           │
   ├─────────────────────────────────────┤
   │ [Image/Video/Poll/Card]             │ <- Media
   ├─────────────────────────────────────┤
   │ [!] Community Note (if any)         │ <- Notes
   ├─────────────────────────────────────┤
   │ [Reply] [Boost] [Fav] [Share]       │ <- Actions
   │  123     456     789                │
   └─────────────────────────────────────┘
   ```

2. **Content Warnings (CW)**:
   - Collapsed by default showing only spoiler text
   - "Show more" button to reveal
   - Remember user's choice per conversation
   - Visual indicator (⚠️) in timeline

3. **Media Attachments**:
   - Up to 4 images in grid layout
   - Video with play button overlay
   - Audio with waveform visualization
   - GIFs auto-play on hover (desktop) or in view (mobile)
   - Alt text indicator and viewer

4. **Threading**:
   - Vertical line connecting replies
   - "Show thread" link for conversations
   - Indentation for nested replies (max 3 levels)
   - Highlight original poster in thread

### 1.4 Posting Interface (Detailed)

**Endpoint**: `POST /api/v1/statuses`
**Docs**: https://docs.joinmastodon.org/methods/statuses/#create

**Complete Compose Structure**:
```typescript
interface ComposeForm {
  // Content
  status: string;                // The text (max 500 chars)
  spoiler_text?: string;         // CW text
  sensitive?: boolean;           // Mark media sensitive
  language?: string;             // ISO 639 language code
  
  // Visibility
  visibility: 'public' | 'unlisted' | 'private' | 'direct';
  
  // Reply Settings
  in_reply_to_id?: string;       // Reply to status
  
  // Media
  media_ids?: string[];          // Uploaded media IDs
  media_attributes?: {
    id: string;
    description?: string;        // Alt text
    focus?: string;              // Focal point "-x,y"
  }[];
  
  // Poll
  poll?: {
    options: string[];           // 2-4 options
    expires_in: number;          // Seconds
    multiple?: boolean;          // Multiple choice
    hide_totals?: boolean;       // Hide results
  };
  
  // Advanced
  scheduled_at?: string;         // ISO 8601 datetime
  
  // Lesser Enhancements
  cost_estimate: {
    immediate: number;
    potential_reach: number;
    viral_scenario: number;
  };
  
  ai_assist: {
    tone_analysis: 'professional' | 'casual' | 'friendly';
    readability_score: number;
    suggested_improvements?: string[];
  };
}
```

**Compose UI Requirements**:

1. **Text Area**:
   - Auto-resize with min 3 rows, max 10 rows
   - Character counter (visual: green → yellow → red)
   - @mention autocomplete with avatars
   - #hashtag autocomplete with usage count
   - :emoji: picker with recent/frequent sections
   - URL shortener preview

2. **Media Upload**:
   - Drag & drop zone
   - Paste from clipboard
   - Progress bars during upload
   - Image editor (crop, rotate, filters)
   - Alt text reminder badge
   - Reorder with drag handles
   - Cost indicator per file

3. **Poll Creator**:
   - Add/remove options (2-4)
   - Character limit per option (50)
   - Duration picker (5m, 30m, 1h, 6h, 24h, 3d, 7d)
   - Preview how poll appears

4. **Visibility Selector**:
   ```
   🌐 Public - Everyone can see
   🔓 Unlisted - Public but not on timelines
   🔒 Followers - Only followers
   ✉️ Direct - Only mentioned users
   ```

5. **Advanced Options** (collapsible):
   - Language selector
   - Schedule post (date/time picker)
   - Federation override
   - Cost limit setting

### 1.5 Notification System (Detailed)

**Endpoint**: `GET /api/v1/notifications`
**Docs**: https://docs.joinmastodon.org/methods/notifications/

**Complete Notification Types**:
```typescript
interface Notification {
  id: string;
  type: NotificationType;
  created_at: string;
  account: Account;              // Who triggered it
  
  // Type-specific data
  status?: Status;               // For mentions, favs, boosts
  report?: Report;               // For moderation
  
  // Lesser Enhancements
  priority: 'high' | 'normal' | 'low';
  cost_impact?: number;          // For viral posts
  trust_relevance?: number;      // Based on relationships
}

type NotificationType = 
  | 'follow'                     // Someone followed you
  | 'follow_request'             // Follow request (locked accounts)
  | 'mention'                    // Mentioned in a post
  | 'reblog'                     // Post was boosted
  | 'favourite'                  // Post was favorited
  | 'poll'                       // Poll ended
  | 'status'                     // Subscribed user posted
  | 'update'                     // Post was edited
  | 'admin.sign_up'              // New user (admins)
  | 'admin.report';              // New report (moderators)
```

**Notification UI Design**:

1. **Notification Groups**:
   ```
   ┌─────────────────────────────────────┐
   │ 🔔 3 people favorited your post     │
   │ [Avatar][Avatar][Avatar] and 2 more │
   │ "Your post content preview..."      │
   │ 5 minutes ago                       │
   └─────────────────────────────────────┘
   ```

2. **Priority Indicators**:
   - Red dot for high priority
   - Bold for unread
   - Muted style for low priority
   - Cost alert badge for viral posts

3. **Quick Actions**:
   - Swipe right: Mark read
   - Swipe left: Options menu
   - Long press: Preview full context
   - Tap: Navigate to relevant content

4. **Filtering Options**:
   ```typescript
   interface NotificationFilters {
     types: NotificationType[];   // Filter by type
     exclude_follows: boolean;
     exclude_favs: boolean;
     exclude_boosts: boolean;
     min_trust_score?: number;
     show_costs: boolean;
   }
   ```

### 1.6 Search & Discovery (Detailed)

**Endpoint**: `GET /api/v2/search`
**Docs**: https://docs.joinmastodon.org/methods/search/

**Search Interface Requirements**:
```typescript
interface SearchQuery {
  q: string;                     // Query string
  type?: 'accounts' | 'hashtags' | 'statuses';
  resolve?: boolean;             // Resolve remote content
  following?: boolean;           // From followed users only
  account_id?: string;           // From specific account
  exclude_unreviewed?: boolean;  // Trending only
  max_id?: string;               // Pagination
  min_id?: string;
  limit?: number;                // Default 20, max 40
  offset?: number;
  
  // Lesser Enhancements
  min_trust?: number;
  max_cost?: number;
  has_notes?: boolean;
  semantic?: boolean;            // AI-powered search
}
```

**Search UI Components**:

1. **Search Bar**:
   - Prominent placement (header on desktop, top on mobile)
   - Search history dropdown
   - Type-ahead suggestions
   - Clear button
   - Filter toggle

2. **Results Layout**:
   ```
   Tabs: [All] [Accounts] [Hashtags] [Posts]
   
   Accounts:
   ┌─────────────────────────────────────┐
   │ [Avatar] Display Name        [Trust] │
   │ @username@instance.social     [✓]   │
   │ Bio preview with keywords highlight  │
   │ 500 followers · Following           │
   └─────────────────────────────────────┘
   
   Hashtags:
   ┌─────────────────────────────────────┐
   │ #hashtag                     [📈]   │
   │ 1.2K posts · Trending up 25%        │
   │ Related: #similar #tags             │
   └─────────────────────────────────────┘
   ```

3. **Advanced Filters** (slide-out panel):
   - Date range picker
   - Language selector
   - Media type filter
   - Trust score slider
   - Cost range selector
   - Instance filter

### 1.7 Settings & Preferences (Complete)

#### Account Settings
**Endpoint**: `GET/PATCH /api/v1/accounts/update_credentials`

```typescript
interface AccountSettings {
  // Privacy
  locked: boolean;               // Require approval
  discoverable: boolean;         // In directory
  indexable: boolean;            // Search engines
  show_collections: boolean;     // Public follows
  
  // Content
  source: {
    privacy: Visibility;         // Default post visibility
    sensitive: boolean;          // Default sensitive
    language: string;            // Default language
  };
  
  // Lesser Additions
  cost_controls: {
    monthly_budget: number;
    pause_at_limit: boolean;
    visibility_on_limit: Visibility;
  };
  
  trust_settings: {
    auto_vouch: string[];        // Auto-vouch list
    min_trust_to_follow: number;
    show_trust_publicly: boolean;
  };
}
```

#### Appearance Settings
```typescript
interface AppearanceSettings {
  theme: 'light' | 'dark' | 'auto' | 'high-contrast';
  font_size: 'small' | 'medium' | 'large';
  color_scheme: string;          // Custom accent color
  reduce_motion: boolean;
  disable_swiping: boolean;
  
  // UI Preferences
  advanced_view: boolean;        // Multi-column
  auto_play_gifs: boolean;
  expand_spoilers: boolean;
  
  // Lesser Additions
  show_cost_indicators: 'always' | 'hover' | 'never';
  trust_badge_style: 'shield' | 'bar' | 'numeric';
  highlight_low_trust: boolean;
}
```

#### Notification Settings
```typescript
interface NotificationSettings {
  // Email notifications
  email: {
    follow: boolean;
    reblog: boolean;
    favourite: boolean;
    mention: boolean;
    digest: boolean;
  };
  
  // Push notifications
  push: {
    follow: boolean;
    reblog: boolean;
    favourite: boolean;
    mention: boolean;
    poll: boolean;
  };
  
  // Lesser Additions
  priority_rules: {
    high_trust_only: boolean;
    min_follower_count: number;
    cost_alerts: boolean;
    viral_threshold: number;
  };
}
```

### 1.8 Moderation Features

#### Filters
**Endpoints**: `GET/POST/PUT/DELETE /api/v2/filters`
**Docs**: https://docs.joinmastodon.org/methods/filters/

```typescript
interface Filter {
  id: string;
  title: string;
  context: FilterContext[];      // Where it applies
  expires_at?: string;
  action: 'warn' | 'hide';
  keywords: FilterKeyword[];
  
  // Lesser Enhancement
  trust_based: {
    apply_below_trust: number;
    exempt_followed: boolean;
  };
}

interface FilterKeyword {
  id: string;
  keyword: string;
  whole_word: boolean;
}
```

**Filter Management UI**:
1. List of active filters with toggle switches
2. Create filter wizard with context selection
3. Keyword manager with whole-word option
4. Expiration date picker
5. Trust-based exemptions (Lesser)

#### Mutes & Blocks
**Endpoints**: 
- Mutes: `GET /api/v1/mutes`
- Blocks: `GET /api/v1/blocks`

**Management Interface**:
```
Tabs: [Muted Users] [Blocked Users] [Blocked Domains]

User List:
┌─────────────────────────────────────┐
│ [Avatar] Username               [X] │
│ Muted on: Jan 20, 2024             │
│ Expires: Never | In 7 days         │
│ [Unmute] [View Profile]            │
└─────────────────────────────────────┘
```

### 1.9 Direct Messages & Conversations

**Docs**: https://docs.joinmastodon.org/methods/conversations/

#### Conversations List
**Endpoint**: `GET /api/v1/conversations`

```typescript
interface Conversation {
  id: string;
  unread: boolean;
  accounts: Account[];           // Participants
  last_status?: Status;          // Last message
  
  // Lesser Enhancement
  encryption_status?: 'none' | 'transport' | 'e2e';
  total_cost?: number;
}
```

**Conversations UI Design**:
```
┌─────────────────────────────────────┐
│ [Avatars] User1, User2         [🔒] │
│ You: Last message preview...        │
│ 2 hours ago · 3 unread             │
└─────────────────────────────────────┘
```

**Features**:
1. **Inbox View**: List of conversations, newest first
2. **Unread Badges**: Number of unread messages
3. **Participant Avatars**: Stack when multiple
4. **Message Preview**: Truncated last message
5. **Search Conversations**: By participant or content

#### Direct Message Thread
**Endpoint**: `GET /api/v1/timelines/direct`
**Docs**: https://docs.joinmastodon.org/methods/timelines/#direct

**Chat Interface Requirements**:
1. **Message Bubbles**: Sender on right, others on left
2. **Typing Indicators**: Real-time via WebSocket
3. **Read Receipts**: Optional, based on privacy settings
4. **Media Support**: Images, videos with previews
5. **Emoji Reactions**: Quick react to messages
6. **Message Actions**: Copy, delete, report

### 1.10 Bookmarks & Favorites

#### Bookmarks
**Endpoints**: 
- `GET /api/v1/bookmarks` - List bookmarked posts
- `POST /api/v1/statuses/:id/bookmark` - Bookmark a post
- `POST /api/v1/statuses/:id/unbookmark` - Remove bookmark

**Docs**: https://docs.joinmastodon.org/methods/bookmarks/

**Bookmarks UI**:
- Private collection only visible to user
- Organized by date bookmarked
- Search within bookmarks
- Export bookmarks feature
- Folders/categories (Lesser enhancement)

#### Favorites
**Endpoints**:
- `GET /api/v1/favourites` - List favorited posts
- `POST /api/v1/statuses/:id/favourite` - Favorite a post
- `POST /api/v1/statuses/:id/unfavourite` - Unfavorite

**Docs**: https://docs.joinmastodon.org/methods/favourites/

**Favorites vs Bookmarks**:
- Favorites are public (notify author)
- Bookmarks are private (no notification)
- UI should clearly distinguish these

### 1.11 OAuth Applications

**Docs**: https://docs.joinmastodon.org/methods/apps/

#### Authorized Apps
**Endpoint**: `GET /api/v1/oauth/authorized_applications`

```typescript
interface AuthorizedApp {
  id: string;
  name: string;
  website?: string;
  scopes: string;                // Space-separated
  created_at: string;
  last_used?: string;            // Lesser enhancement
  
  // Usage stats (Lesser)
  api_calls_today?: number;
  data_accessed?: string[];
}
```

**App Management UI**:
```
┌─────────────────────────────────────┐
│ [Icon] App Name                     │
│ website.com                         │
│ Scopes: read write follow           │
│ Last used: 2 hours ago              │
│ [Revoke Access]                     │
└─────────────────────────────────────┘
```

#### Creating Apps
**Endpoint**: `POST /api/v1/apps`
**Docs**: https://docs.joinmastodon.org/methods/apps/#create

Required for third-party clients to authenticate.

### 1.12 Import & Export

**Docs**: https://docs.joinmastodon.org/methods/accounts/#import-export

#### Data Export
**Endpoint**: `POST /api/v1/exports`

```typescript
interface ExportRequest {
  type: 'followers' | 'following' | 'mutes' | 'blocks' | 'lists' | 'bookmarks';
  format?: 'csv' | 'json';       // Lesser enhancement
}
```

**Export UI**:
1. **Data Selection**: Checkboxes for each type
2. **Format Choice**: CSV for compatibility, JSON for full data
3. **Progress Bar**: For large exports
4. **Email Notification**: When ready
5. **Cost Warning**: For large media exports

#### Data Import
**Endpoint**: `POST /api/v1/import`

**Import UI Requirements**:
1. **File Upload**: Drag & drop or browse
2. **Preview**: Show what will be imported
3. **Conflict Resolution**: Handle duplicates
4. **Progress Tracking**: Real-time updates
5. **Rollback Option**: Undo recent import

### 1.13 Account Security

**Docs**: https://docs.joinmastodon.org/methods/accounts/#security

#### Two-Factor Authentication
**Endpoints**:
- `GET /api/v1/accounts/2fa` - Get 2FA status
- `POST /api/v1/accounts/2fa/enable` - Enable 2FA
- `POST /api/v1/accounts/2fa/disable` - Disable 2FA

```typescript
interface TwoFactorSettings {
  enabled: boolean;
  confirmed: boolean;
  provisioning_uri?: string;      // For QR code
  backup_codes?: string[];        // One-time codes
  
  // Lesser Enhancement
  trusted_devices?: {
    id: string;
    name: string;
    last_used: string;
    browser: string;
    location?: string;
  }[];
}
```

**2FA Setup Flow**:
1. **Enable 2FA**: Generate secret
2. **QR Code Display**: For authenticator apps
3. **Verification**: Enter code to confirm
4. **Backup Codes**: Display and require acknowledgment
5. **Recovery Options**: Alternative methods

### 1.14 Instance Features

**Docs**: https://docs.joinmastodon.org/methods/instance/

#### Instance Information
**Endpoint**: `GET /api/v2/instance`

```typescript
interface InstanceInfo {
  // Standard Mastodon
  domain: string;
  title: string;
  version: string;
  source_url: string;
  description: string;
  usage: {
    users: {
      active_month: number;
    };
  };
  thumbnail: {
    url: string;
  };
  languages: string[];
  configuration: {
    statuses: {
      max_characters: number;
      max_media_attachments: number;
    };
    media_attachments: {
      supported_mime_types: string[];
      image_size_limit: number;
      video_size_limit: number;
    };
    polls: {
      max_options: number;
      max_characters_per_option: number;
    };
  };
  
  // Lesser Enhancements
  cost_transparency: {
    base_monthly_cost: number;
    cost_per_user: number;
    funding_status: 'funded' | 'needs_support' | 'critical';
  };
  
  moderation_transparency: {
    total_reports_week: number;
    average_resolution_time: number;
    moderator_count: number;
  };
}
```

**Instance Info Display**:
1. **About Page**: Rich instance information
2. **Rules Display**: Clear community guidelines
3. **Statistics**: Active users, posts, federation
4. **Cost Transparency**: Running costs, funding
5. **Contact Info**: Admin contacts

#### Instance Rules
**Endpoint**: `GET /api/v1/instance/rules`
**Docs**: https://docs.joinmastodon.org/methods/instance/#rules

Display prominently during signup and in help.

### 1.15 Scheduled Posts

**Docs**: https://docs.joinmastodon.org/methods/scheduled_statuses/

**Endpoints**:
- `GET /api/v1/scheduled_statuses` - List scheduled
- `GET /api/v1/scheduled_statuses/:id` - Get specific
- `PUT /api/v1/scheduled_statuses/:id` - Update
- `DELETE /api/v1/scheduled_statuses/:id` - Cancel

```typescript
interface ScheduledStatus {
  id: string;
  scheduled_at: string;          // ISO 8601
  params: {
    text: string;
    visibility: Visibility;
    media_ids?: string[];
    poll?: PollParams;
  };
  
  // Lesser Enhancement
  estimated_cost: number;
  optimal_time_suggestion?: string;
}
```

**Scheduled Posts UI**:
```
┌─────────────────────────────────────┐
│ 📅 Scheduled for Jan 20, 2:00 PM    │
│ "Post content preview..."           │
│ [Edit] [Delete] [Post Now]          │
└─────────────────────────────────────┘
```

### 1.16 Trends & Discovery

**Docs**: https://docs.joinmastodon.org/methods/trends/

#### Trending Hashtags
**Endpoint**: `GET /api/v1/trends/tags`

```typescript
interface TrendingTag {
  name: string;
  url: string;
  history: {
    day: string;
    accounts: string;
    uses: string;
  }[];
  
  // Lesser Enhancement
  sentiment_analysis?: {
    positive: number;
    neutral: number;
    negative: number;
  };
}
```

#### Trending Links
**Endpoint**: `GET /api/v1/trends/links`
**Docs**: https://docs.joinmastodon.org/methods/trends/#links

Preview cards for shared URLs with engagement metrics.

#### Suggested Follows
**Endpoint**: `GET /api/v2/suggestions`
**Docs**: https://docs.joinmastodon.org/methods/suggestions/

**Recommendation UI**:
1. **Reason for Suggestion**: "Followed by X, Y"
2. **Preview Bio**: First 100 chars
3. **Quick Follow**: Without leaving page
4. **Dismiss Option**: Remove from suggestions
5. **Trust Score**: Lesser enhancement

### 1.17 Streaming API

**Docs**: https://docs.joinmastodon.org/methods/streaming/

#### WebSocket Connection
```typescript
// Establish connection
const ws = new WebSocket('wss://instance.com/api/v1/streaming');

// Subscribe to streams
ws.send(JSON.stringify({
  type: 'subscribe',
  stream: 'user'
}));

// Available streams
type StreamType = 
  | 'user'              // Home timeline + notifications
  | 'public'            // Public timeline
  | 'public:local'      // Local public timeline
  | 'hashtag'           // Specific hashtag
  | 'list'              // List timeline
  | 'direct'            // Direct messages;
```

**Real-time Features**:
1. **Live Timeline Updates**: New posts appear
2. **Notification Badges**: Instant updates
3. **Typing Indicators**: In DMs
4. **Online Status**: For mutuals
5. **Live Counters**: Likes/boosts in real-time

### 1.18 Admin Interface

**Docs**: https://docs.joinmastodon.org/methods/admin/

For users with admin/moderator privileges:

#### Reports Management
**Endpoint**: `GET /api/v1/admin/reports`

```typescript
interface Report {
  id: string;
  action_taken: boolean;
  action_taken_at?: string;
  category: 'spam' | 'violation' | 'other';
  comment: string;
  forwarded: boolean;
  created_at: string;
  status_ids: string[];
  rule_ids: string[];
  target_account: Account;
  
  // Lesser Enhancements
  ai_analysis: {
    severity_score: number;
    suggested_action: string;
    similar_reports: number;
  };
  
  moderation_consensus?: {
    reviewers: string[];
    votes: Record<string, 'approve' | 'reject'>;
    confidence: number;
  };
}
```

**Moderation Queue UI**:
1. **Priority Sorting**: By severity/age
2. **Bulk Actions**: Handle multiple reports
3. **Context Display**: Full conversation thread
4. **Quick Actions**: Warn, suspend, dismiss
5. **AI Assistance**: Suggested responses

### 1.19 Accessibility Features

**Docs**: https://docs.joinmastodon.org/methods/statuses/#create (see media_attributes)

#### Alt Text Requirements
```typescript
interface MediaAccessibility {
  description: string;            // Alt text (required)
  focus?: string;                 // Focal point for cropping
  
  // Lesser Enhancements
  auto_generated_alt?: string;    // AI suggestion
  alt_quality_score?: number;     // How descriptive
  translation_available?: boolean;
}
```

**Accessibility UI**:
1. **Alt Text Reminders**: Before posting
2. **AI Suggestions**: Auto-generate descriptions
3. **Quality Indicators**: Rate alt text quality
4. **Screen Reader Mode**: Optimized interface
5. **High Contrast**: Theme option

### 1.20 Mobile-Specific Considerations

**Touch Targets**: Minimum 44x44px
**Gesture Support**:
- Swipe between tabs
- Pull to refresh
- Long press for previews
- Pinch to zoom images

**Offline Support**:
- Cache recent timeline
- Queue posts for sending
- Show cached profiles
- Offline indicator

**Performance**:
- Virtual scrolling for long lists
- Progressive image loading
- Reduced data mode option
- Background sync

## Part 2: Advanced Features Dashboard

### Additional Dashboard Features

#### Analytics Dashboard
**Endpoint**: `GET /api/v1/analytics`
**Docs**: Custom Lesser endpoint

```typescript
interface AnalyticsDashboard {
  // Engagement metrics
  posts_created: number;
  total_interactions: number;
  average_reach: number;
  
  // Growth metrics
  new_followers: number;
  follower_churn: number;
  
  // Cost metrics
  total_cost_mtd: number;
  cost_per_interaction: number;
  storage_used_gb: number;
  
  // Federation metrics
  unique_instances_reached: number;
  federation_success_rate: number;
}
```

**Visualization Requirements**:
1. **Time Series Charts**: Engagement over time
2. **Cost Breakdown**: Pie chart by operation type
3. **Federation Map**: Geographic distribution
4. **Follower Analytics**: Growth and demographics
5. **Content Performance**: Best performing posts

## API Reference Links

### Core Documentation
- **Mastodon API**: https://docs.joinmastodon.org/api/
- **OAuth**: https://docs.joinmastodon.org/spec/oauth/
- **ActivityPub**: https://www.w3.org/TR/activitypub/
- **WebFinger**: https://docs.joinmastodon.org/spec/webfinger/

### Method References
- **Accounts**: https://docs.joinmastodon.org/methods/accounts/
- **Statuses**: https://docs.joinmastodon.org/methods/statuses/
- **Timelines**: https://docs.joinmastodon.org/methods/timelines/
- **Notifications**: https://docs.joinmastodon.org/methods/notifications/
- **Search**: https://docs.joinmastodon.org/methods/search/
- **Instance**: https://docs.joinmastodon.org/methods/instance/
- **Streaming**: https://docs.joinmastodon.org/methods/streaming/
- **Lists**: https://docs.joinmastodon.org/methods/lists/
- **Filters**: https://docs.joinmastodon.org/methods/filters/

### Client Implementation Guides
- **Authentication**: https://docs.joinmastodon.org/client/token/
- **Entities**: https://docs.joinmastodon.org/entities/
- **Error Handling**: https://docs.joinmastodon.org/api/guidelines/#error-handling
- **Rate Limits**: https://docs.joinmastodon.org/api/rate-limits/

## Design System Requirements

### Typography
```css
/* Headings */
--font-display: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto;
--font-body: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto;
--font-mono: "SF Mono", Monaco, Consolas, monospace;

/* Sizes */
--text-xs: 0.75rem;    /* 12px */
--text-sm: 0.875rem;   /* 14px */
--text-base: 1rem;     /* 16px */
--text-lg: 1.125rem;   /* 18px */
--text-xl: 1.25rem;    /* 20px */
--text-2xl: 1.5rem;    /* 24px */
```

### Spacing System
```css
/* Based on 4px grid */
--space-1: 0.25rem;    /* 4px */
--space-2: 0.5rem;     /* 8px */
--space-3: 0.75rem;    /* 12px */
--space-4: 1rem;       /* 16px */
--space-5: 1.25rem;    /* 20px */
--space-6: 1.5rem;     /* 24px */
--space-8: 2rem;       /* 32px */
```

### Animation Guidelines
```css
/* Consistent timing */
--duration-fast: 150ms;
--duration-normal: 250ms;
--duration-slow: 350ms;

/* Easing */
--ease-out: cubic-bezier(0.0, 0, 0.2, 1);
--ease-in-out: cubic-bezier(0.4, 0, 0.2, 1);

/* Reduce motion support */
@media (prefers-reduced-motion: reduce) {
  * { animation-duration: 0.01ms !important; }
}
```

## Implementation Checklist

### Phase 1: Core Features (Weeks 1-2)
- [ ] User profiles with trust indicators
- [ ] Timeline views with cost tracking
- [ ] Basic compose functionality
- [ ] Follow/unfollow system
- [ ] Simple notifications

### Phase 2: Enhanced Features (Weeks 3-4)
- [ ] Lists management
- [ ] Advanced search with AI
- [ ] Media attachments
- [ ] Polls support
- [ ] Settings panels

### Phase 3: Moderation & Trust (Weeks 5-6)
- [ ] Filter system
- [ ] Mute/block management
- [ ] Community notes display
- [ ] Trust visualizations
- [ ] Moderation queue

### Phase 4: Polish & Optimization (Weeks 7-8)
- [ ] Mobile optimizations
- [ ] Offline support
- [ ] Performance tuning
- [ ] Accessibility audit
- [ ] Animation polish

---

*This comprehensive guide should enable the Greater UI team to build a fully-featured Mastodon client that seamlessly integrates Lesser's innovations. Remember: familiarity first, innovation second.* 