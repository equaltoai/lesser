# Lesser Instance Configuration Guide

This guide explains how to configure your Lesser instance settings.

## Configuration Types

### 1. Static Configuration (Environment Variables)

Set these in your infrastructure deployment (`infra/main.go`):

```go
"INSTANCE_TITLE":       pulumi.String("My Lesser Instance"),
"INSTANCE_SHORT_DESC":  pulumi.String("A personal ActivityPub server"),
"INSTANCE_DESCRIPTION": pulumi.String("Welcome to my personal fediverse instance"),
"INSTANCE_ADMIN_EMAIL": pulumi.Sprintf("admin@%s", domain),
"REGISTRATIONS_OPEN":   pulumi.String("false"),
"APPROVAL_REQUIRED":    pulumi.String("true"),
"INVITES_ENABLED":      pulumi.String("false"),
"FEDERATION_ENABLED":   pulumi.String("true"),
```

After changing these, redeploy with:
```bash
cd infra && pulumi up
```

### 2. Dynamic Configuration (DynamoDB)

Use the `configure-instance` tool to set rules and extended description:

#### Build the tool:
```bash
make build-configure-instance
```

#### Set Instance Rules:
```bash
./bin/configure-instance -set-rules "Be respectful to others,No spam or advertising,Follow local laws,No hate speech"
```

#### Set Extended Description:
```bash
./bin/configure-instance -set-description "<h1>Welcome to My Instance</h1>
<p>This is a personal ActivityPub server running <a href='https://github.com/equaltoai/lesser'>Lesser</a>.</p>
<h2>About</h2>
<p>This instance is for personal use. Registration is currently closed.</p>
<h2>Contact</h2>
<p>For any questions, please email admin@lesser.host</p>"
```

#### View Current Configuration:
```bash
./bin/configure-instance -show
```

## Example Configurations

### Personal Instance
```bash
# Environment variables (in infra/main.go)
"INSTANCE_TITLE": "John's Fediverse"
"INSTANCE_SHORT_DESC": "Personal instance of John Doe"
"REGISTRATIONS_OPEN": "false"

# Rules
./bin/configure-instance -set-rules "This is a personal instance,No registrations accepted"
```

### Small Community Instance
```bash
# Environment variables
"INSTANCE_TITLE": "Tech Enthusiasts"
"INSTANCE_SHORT_DESC": "A small community for tech discussions"
"REGISTRATIONS_OPEN": "true"
"APPROVAL_REQUIRED": "true"

# Rules
./bin/configure-instance -set-rules "Be kind and respectful,Stay on topic (tech),No spam or self-promotion,English only,NSFW content must be marked"
```

### Open Instance
```bash
# Environment variables
"INSTANCE_TITLE": "OpenVerse"
"INSTANCE_SHORT_DESC": "An open ActivityPub instance"
"REGISTRATIONS_OPEN": "true"
"APPROVAL_REQUIRED": "false"

# Rules
./bin/configure-instance -set-rules "No illegal content,No harassment or hate speech,No spam,Mark NSFW content appropriately,No impersonation"
```

## HTML Formatting Tips

The extended description supports HTML. Here are some examples:

### Basic formatting:
```html
<h1>Main Title</h1>
<h2>Section Title</h2>
<p>Regular paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
<ul>
  <li>Bullet point 1</li>
  <li>Bullet point 2</li>
</ul>
```

### Links:
```html
<p>Visit our <a href="https://example.com">website</a> for more info.</p>
```

### Complete example:
```html
<h1>Welcome to TechTalk Instance</h1>
<p>A focused community for technology enthusiasts.</p>

<h2>What We're About</h2>
<p>We discuss programming, hardware, open source, and digital privacy.</p>

<h2>Code of Conduct</h2>
<ul>
  <li>Be respectful and constructive</li>
  <li>No off-topic posts</li>
  <li>Credit sources and respect licenses</li>
</ul>

<h2>Resources</h2>
<p>
  <a href="https://docs.joinmastodon.org">Mastodon Guide</a> |
  <a href="https://github.com/equaltoai/lesser">Lesser on GitHub</a>
</p>
```

## Applying Changes

1. **Environment Variables**: Require redeployment
   ```bash
   cd infra && pulumi up
   ```

2. **Rules & Extended Description**: Take effect immediately
   ```bash
   ./bin/configure-instance -set-rules "..."
   ./bin/configure-instance -set-description "..."
   ```

3. **Verify in Mastodon clients**: 
   - Rules appear in sign-up flow
   - Extended description appears in "About this server" 