# Quick Start: Deploy Lesser in 15 Minutes

Deploy your own ActivityPub instance at 1/100th the cost of traditional hosting. This guide will have you up and running in 15 minutes!

## Prerequisites

Before starting, ensure you have:

- ✅ **AWS Account** with billing enabled
- ✅ **Pulumi Account** (free tier is fine)
- ✅ **Go 1.21+** installed
- ✅ **Git** installed
- ✅ **Domain name** (for federation)

### Quick Setup Commands

```bash
# Install Pulumi (if not already installed)
curl -fsSL https://get.pulumi.com | sh

# Configure AWS CLI (if not already configured)
aws configure

# Verify installations
go version        # Should show 1.21 or higher
pulumi version    # Should show latest version
aws sts get-caller-identity  # Should show your AWS account
```

## 🚀 Deployment Steps

### 1. Clone Lesser (1 minute)

```bash
git clone https://github.com/yourusername/lesser
cd lesser
```

### 2. Configure Your Instance (2 minutes)

```bash
# Copy the example configuration
cp instance.config.example.yaml instance.config.yaml

# Edit with your settings
nano instance.config.yaml
```

**Minimal configuration:**

```yaml
instance:
  domain: "your-domain.com"  # Your domain name
  name: "My Lesser Instance"  # Display name
  description: "A modern ActivityPub server"
  
admin:
  username: "admin"          # Your admin username
  email: "admin@your-domain.com"
  
features:
  open_registration: false   # Start closed, open later
```

### 3. Deploy Infrastructure (10 minutes)

```bash
# Initialize Pulumi stack
pulumi stack init production

# Set your AWS region
pulumi config set aws:region us-east-1

# Deploy!
make deploy
```

**What's being created:**
- Lambda functions for all endpoints
- DynamoDB tables for data storage
- S3 buckets for media
- CloudFront distribution
- API Gateway
- Route53 DNS records

### 4. Configure DNS (1 minute)

If your domain is NOT in Route53, add these records to your DNS provider:

```
Type  Name              Value
A     @                 <CloudFront distribution domain>
A     www               <CloudFront distribution domain>
CNAME api               <API Gateway domain>
```

The exact values will be shown after deployment completes.

### 5. Initialize Instance (1 minute)

```bash
# Run the configuration script
./configure-instance

# This will:
# - Create admin account
# - Set up federation keys
# - Configure instance settings
# - Test the deployment
```

## ✅ Verification

Your instance is ready when you can:

1. **Visit your domain** and see the Lesser welcome page
2. **Check federation** is working:
   ```bash
   curl https://your-domain.com/.well-known/webfinger?resource=acct:admin@your-domain.com
   ```
3. **Log in** with any Mastodon app using your domain

## 📱 Connect with Mastodon Apps

Lesser is 100% compatible with Mastodon apps:

### iOS
- [Ivory](https://tapbots.com/ivory/)
- [Ice Cubes](https://geticecu.be/)
- [Toot!](https://apps.apple.com/app/toot/id1229021451)

### Android
- [Tusky](https://tusky.app/)
- [Fedilab](https://fedilab.app/)
- [Megalodon](https://sk22.github.io/megalodon/)

### Web
- Use any Mastodon web client
- Or the built-in Lesser web interface

**To connect:** Enter your domain (e.g., `your-domain.com`) when the app asks for an instance.

## 💰 Cost Estimates

Your actual costs will depend on usage, but here's what to expect:

| Monthly Active Users | Estimated Monthly Cost |
|---------------------|----------------------|
| 1-10 users         | $0.50 - $2           |
| 10-100 users       | $2 - $10             |
| 100-1000 users     | $10 - $50            |

**First month:** Expect ~$5 in AWS costs for initial setup and testing.

## 🛠️ Common Issues

### "Deployment failed with permissions error"
```bash
# Ensure your AWS credentials have admin access
aws iam get-user
```

### "Domain not resolving"
- DNS changes can take up to 48 hours
- Use `dig your-domain.com` to check propagation

### "Can't connect with Mastodon app"
- Ensure HTTPS is working: `curl https://your-domain.com`
- Check federation endpoint: `curl https://your-domain.com/.well-known/nodeinfo`

### "DynamoDB throttling errors"
- Normal during initial setup
- Auto-scaling will adjust within minutes

## 🎯 Next Steps

### Essential Configuration

1. **Set Up Your Instance** - Configure your instance settings
2. **Enable Moderation** - Keep your instance safe
3. **Monitor Performance** - Track your instance health

### Customize Your Instance

1. **[Configure Your Instance](INSTANCE_CONFIG_EXAMPLE.md)** - See configuration examples
2. **Add Custom Features** - Extend your instance
3. **Set Community Guidelines** - Define your instance rules

### Advanced Features

1. **[Enable AI Features](../architecture/AI_INTEGRATION.md)** - Semantic search & translation
2. **[Understand the Architecture](../architecture/OVERVIEW.md)** - Deep dive into Lesser
3. **[Review Security](../security/)** - Security best practices

## 🆘 Getting Help

- **Discord**: [Join our community](https://discord.gg/lesser)
- **Issues**: [GitHub Issues](https://github.com/yourusername/lesser/issues)
- **Docs**: [Full documentation](../README.md)

## 🎉 Congratulations!

You now have your own ActivityPub instance running at a fraction of traditional hosting costs!

**Share your instance**: Tag us on Mastodon [@lesser@mastodon.social](https://mastodon.social/@lesser) 

**Star the project**: If Lesser saved you money, [star us on GitHub](https://github.com/yourusername/lesser)!

---

<div align="center">

**Problems?** We're here to help in [Discord](https://discord.gg/lesser)

[Back to Docs](../README.md) • [Architecture Overview](../architecture/OVERVIEW.md) • [API Reference](../api/QUICK_REFERENCE.md)

</div> 