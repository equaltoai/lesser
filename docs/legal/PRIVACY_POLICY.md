# Privacy Policy for Lesser

*Last Updated: June 6 2025*

## Introduction

Welcome to Lesser, a revolutionary serverless ActivityPub platform that makes federated social media essentially free to operate. This Privacy Policy explains how we collect, use, share, and protect your information when you use Lesser.

Lesser is built on principles of transparency, community governance, and user empowerment. We believe you should know exactly what happens to your data, how much it costs to process, and who can see it.

## Key Privacy Features

- **Cost Transparency**: Every operation shows its data processing cost
- **Community Moderation**: Moderation decisions are made by consensus, not algorithms
- **Portable Data**: Your reputation and content can move with you
- **Federation**: Your data may be shared with other ActivityPub servers
- **AI Transparency**: We tell you when and how AI analyzes your content

## Information We Collect

### 1. Account Information
When you create an account, we collect:
- Username (must be unique within the instance)
- Email address (for account recovery and notifications)
- Display name and bio (optional)
- Avatar and header images (optional)
- Language preferences
- Timezone settings

### 2. Content You Create
- Posts (called "notes" or "statuses")
- Replies and conversations
- Media attachments (images, videos, audio)
- Community Notes you contribute
- Polls and votes
- Lists and bookmarks

### 3. Social Graph Data
- Accounts you follow
- Accounts that follow you
- Accounts you block or mute
- Trust relationships you establish
- Moderation reviews you submit

### 4. Automatically Collected Information
- IP address (for rate limiting and security)
- User agent (browser/app information)
- ActivityPub interactions from other servers
- Timestamps of activities
- Cost metrics for operations you perform

### 5. Federation Data
When you interact with users on other servers:
- Your public profile information
- Public posts and interactions
- Follow relationships with remote users

## How We Use Your Information

### 1. To Provide Core Services
- Create and maintain your account
- Display your content to authorized viewers
- Enable social interactions (follows, likes, boosts)
- Process media uploads and attachments
- Calculate and display cost transparency metrics

### 2. For Federation
- Share your public content with other ActivityPub servers
- Receive content from users you follow on other servers
- Verify cryptographic signatures for secure federation
- Maintain inbox/outbox collections per ActivityPub spec

### 3. For Safety and Moderation
- **Reactive Moderation Mesh**: Your content may be reviewed by trusted community members
- **AI Pre-Screening**: We use AWS services to detect:
  - Potential spam or harmful content
  - NSFW images (via AWS Rekognition)
  - Toxic text (via AWS Comprehend)
  - AI-generated content (via AWS Bedrock)
- **Trust Graph**: Calculate trust scores based on community interactions
- **Consensus Building**: Enable community-driven moderation decisions

### 4. For Enhanced Features
- **Search**: Index your public content for search
- **Recommendations**: Suggest accounts to follow
- **Analytics**: Provide insights about your content reach
- **Community Notes**: Enable collaborative fact-checking
- **Portable Reputation**: Calculate and sign your reputation score

### 5. For Platform Improvement
- Monitor system performance and reliability
- Track feature usage (anonymized)
- Debug technical issues
- Plan capacity and scaling

## Data Sharing and Disclosure

### 1. Federation (Most Important!)
**By design, Lesser shares your public content with the federated network:**
- Public posts are shared with all followers' servers
- Unlisted posts are shared with mentioned users' servers
- Your public profile is accessible to any ActivityPub server
- Servers you've never heard of may have copies of your public content
- **This sharing cannot be undone** - the nature of federation

### 2. Service Providers
We use these AWS services which process your data:
- **AWS Lambda**: Serverless compute (processes all requests)
- **Amazon DynamoDB**: Primary data storage
- **Amazon S3**: Media file storage
- **Amazon CloudFront**: Content delivery network
- **AWS Comprehend**: Text analysis for moderation
- **AWS Rekognition**: Image analysis for moderation
- **AWS Bedrock**: AI content detection and embeddings
- **Amazon OpenSearch**: Full-text search capabilities

### 3. Community Moderators
When content is flagged:
- Trusted community reviewers can see the content
- Reviews are weighted by reviewer trust scores
- All moderation actions are logged and transparent
- You can appeal decisions through the consensus system

### 4. Legal Requirements
We may disclose information if required by:
- Valid legal process (subpoena, court order)
- To protect safety of users or the public
- To protect our rights and property
- During business transfers or acquisitions

### 5. With Your Consent
- When you explicitly authorize sharing
- When you participate in public features
- When you enable specific integrations

## Your Privacy Controls

### 1. Visibility Settings
- **Public**: Visible to everyone, federated widely
- **Unlisted**: Not in public timelines, but accessible via URL
- **Followers-only**: Only your followers can see
- **Mentioned-only**: Only mentioned users can see

### 2. Account Controls
- Make your account private (approve followers manually)
- Block or mute accounts and entire domains
- Control who can message you
- Manage notification preferences
- Delete your account and content

### 3. Data Portability
- Export all your data in ActivityPub format
- Download your archive anytime
- Transfer your reputation score to another instance
- Import data from other ActivityPub servers

### 4. AI and Automation Controls
- Opt out of AI analysis for your content
- Disable automated content warnings
- Choose human-only moderation review
- Control search indexing of your content

## Data Retention

### 1. Active Account Data
- Content: Retained until you delete it
- Account info: Retained while account is active
- Media: Retained unless orphaned (unused for 90 days)
- Cost metrics: Aggregated after 30 days

### 2. Deleted Content
- Removed from our primary database immediately
- Deletion federated to other servers (best effort)
- May persist in backups for up to 30 days
- **Cannot guarantee deletion from other servers**

### 3. Moderation Records
- Kept for 180 days for appeals and transparency
- Anonymized after retention period
- Statistical data retained indefinitely

### 4. Technical Logs
- API access logs: 30 days
- Error logs: 90 days
- Performance metrics: Aggregated after 7 days
- Cost tracking: Detailed for 30 days, aggregated after

## Data Security

### 1. Technical Measures
- Encryption in transit (TLS 1.3)
- Encryption at rest (DynamoDB, S3)
- Private keys stored in AWS KMS
- Regular security updates and patches

### 2. Access Controls
- Multi-factor authentication available
- OAuth 2.0 with PKCE for third-party apps
- Granular permission scopes
- Regular access reviews

### 3. Incident Response
- 24-hour breach notification commitment
- Transparent incident reporting
- Affected user notifications
- Remediation documentation

## International Data Transfers

Lesser operates globally through AWS infrastructure:
- Data may be processed in any AWS region
- We rely on AWS's compliance frameworks
- EU-US Data Privacy Framework participant (via AWS)
- Standard contractual clauses where applicable

## Children's Privacy

- Lesser is not intended for users under 13
- We don't knowingly collect data from children
- Parents may contact us to remove children's data
- Age verification may be required in some jurisdictions

## AI and Automated Processing

### 1. What We Use AI For
- Content moderation pre-screening
- Spam detection
- Search improvements
- Content recommendations
- Language translation
- Accessibility features

### 2. Your Rights Regarding AI
- Know when AI analyzes your content
- Request human review of AI decisions
- Opt out of certain AI features
- Access AI-generated insights about your content

### 3. AI Transparency
- We use AWS AI services, not custom models
- No training on user data without consent
- Clear labeling of AI-generated features
- Cost tracking for AI operations

## Your Rights

### 1. Access and Portability
- Download all your data
- Receive data in machine-readable format
- Transfer to another service
- Know what data we have

### 2. Correction and Deletion
- Edit your content anytime
- Delete specific posts or entire account
- Correct inaccurate information
- Request removal from search indexes

### 3. Control and Consent
- Granular privacy controls
- Withdraw consent for processing
- Object to specific uses
- Restrict processing

### 4. Transparency and Information
- Know how your data is used
- See cost of data operations
- View moderation decisions
- Access federation logs

## Lesser-Specific Privacy Features

### 1. Cost Transparency
- Every operation shows its data processing cost
- Monthly cost summaries available
- Understand the real cost of privacy
- Make informed decisions about usage

### 2. Community Notes
- Contribute fact-checks to public posts
- Notes are public and attributed
- Voting is recorded but weighted by reputation
- Can delete your notes anytime

### 3. Trust Graph
- Your trust relationships are private by default
- Trust scores affect moderation weight
- Can make trust relationships public
- Portable reputation includes trust metrics

### 4. Reactive Moderation Mesh
- Moderation decisions by community consensus
- Your reviews affect trust scores
- All actions are logged and auditable
- Appeals process is transparent

## Contact Us

### Data Protection Officer
protection@lesser.social

### Privacy Questions
privacy@lesser.social

### Legal Requests
legal@lesser.social

### Bug Bounty
security@lesser.social

## Changes to This Policy

We'll notify you of changes by:
- Email to registered users
- Prominent notice on the platform
- 30-day advance notice for material changes

## Jurisdiction-Specific Rights

### European Union (GDPR)
- Right to object to processing
- Right to restriction
- Data Protection Authority complaints
- Legal basis documentation available

### California (CCPA/CPRA)
- Do Not Sell controls
- Category disclosure available
- Service provider list available
- Annual privacy rights metrics

### Other Jurisdictions
- We comply with applicable local laws
- Contact us for jurisdiction-specific rights
- VPN usage is permitted
- No discrimination for privacy choices

---

*This privacy policy is licensed under CC BY-SA 4.0. You may adapt it for your own Lesser instance with attribution.* 