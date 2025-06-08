# k6 Load Testing Setup Guide

This guide will help you set up and run k6 load tests for Lesser, both locally and on Grafana Cloud k6.

## Prerequisites

1. **Install k6** (if not already installed):
   ```bash
   # macOS
   brew install k6
   
   # Linux
   sudo gpg -k
   sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
   echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
   sudo apt-get update
   sudo apt-get install k6
   ```

2. **Get your Grafana Cloud k6 credentials**:
   - Log in to [Grafana Cloud](https://grafana.com)
   - Navigate to k6 Cloud (Performance Testing)
   - Go to Account Settings → API Tokens
   - Create a new API token
   - Note your Project ID from the project settings

## Setup

1. **Authenticate with Grafana Cloud**:
   ```bash
   k6 cloud login -t YOUR_API_TOKEN
   ```

2. **Set up environment variables** (create a `.env.k6` file):
   ```bash
   # Lesser instance configuration
   export LESSER_URL="https://your-lesser-instance.com"
   export LESSER_TOKEN="your-access-token"
   
   # Grafana Cloud k6 configuration
   export K6_PROJECT_ID="your-project-id"
   export K6_CLOUD_TOKEN="your-api-token"
   ```

3. **Load the environment variables**:
   ```bash
   source .env.k6
   ```

## Running Tests

### Local Execution

Run the test locally and view results in your terminal:

```bash
k6 run tests/load/lesser_load_test.js
```

### Local Execution with Cloud Output

Run the test locally but stream results to Grafana Cloud:

```bash
k6 run -o cloud tests/load/lesser_load_test.js
```

### Cloud Execution

Run the test entirely on Grafana Cloud's infrastructure:

```bash
k6 cloud run tests/load/lesser_load_test.js
```

## Customizing Test Parameters

You can override test settings via command line:

```bash
# Run with different stages
k6 run --stage 30s:10,1m:20,30s:0 tests/load/lesser_load_test.js

# Run with different duration
k6 run --duration 5m --vus 50 tests/load/lesser_load_test.js

# Run with custom environment variables
k6 run --env LESSER_URL=https://staging.lesser.com tests/load/lesser_load_test.js
```

## Understanding the Test

The Lesser load test simulates realistic user behavior:

- **30% of users**: Read home timeline
- **20% of users**: Read public timeline
- **20% of users**: Create and delete statuses
- **15% of users**: Search for content
- **15% of users**: Check notifications

The test includes:
- Gradual ramp-up and ramp-down
- Spike testing (200 users)
- Performance thresholds (95% < 500ms)
- Error rate monitoring
- Cost tracking (via Lesser's custom headers)

## Viewing Results

### In Terminal (Local Runs)
You'll see real-time metrics including:
- Request rate
- Response times (avg, p95, p99)
- Error rate
- Data transfer

### In Grafana Cloud
1. Log in to Grafana Cloud
2. Navigate to k6 Cloud
3. Find your test run
4. View detailed metrics:
   - Performance Overview
   - HTTP Timings
   - Checks & Thresholds
   - Custom Metrics (cost per request)

## Best Practices

1. **Start Small**: Begin with lower user counts to validate your test
2. **Monitor Costs**: Watch the AWS cost headers during tests
3. **Clean Test Data**: The test automatically deletes created statuses
4. **Use Staging**: Test against staging environments first
5. **Schedule Tests**: Avoid peak hours for production tests

## Troubleshooting

### Authentication Issues
```bash
# Check stored token
k6 cloud login -s

# Reset and re-authenticate
k6 cloud login -r
k6 cloud login -t YOUR_NEW_TOKEN
```

### Connection Issues
```bash
# Test instance availability
curl -I https://your-lesser-instance.com/api/v1/instance

# Test with verbose output
k6 run --http-debug tests/load/lesser_load_test.js
```

### Performance Issues
- Reduce the number of VUs (virtual users)
- Increase ramp-up time
- Check network connectivity
- Verify instance capacity

## Advanced Configuration

### Multiple Load Zones
Update the test file to distribute load across regions:

```javascript
distribution: {
  'amazon:us:ashburn': { loadZone: 'amazon:us:ashburn', percent: 50 },
  'amazon:eu:dublin': { loadZone: 'amazon:eu:dublin', percent: 30 },
  'amazon:ap:tokyo': { loadZone: 'amazon:ap:tokyo', percent: 20 }
}
```

### Custom Metrics
The test already tracks:
- Error rate
- Cost per request (from Lesser headers)

Add more custom metrics as needed in the test file.

## Next Steps

1. Run a small test to validate setup
2. Review initial results in Grafana Cloud
3. Adjust test parameters based on findings
4. Create baseline performance metrics
5. Set up regular test runs or CI/CD integration 