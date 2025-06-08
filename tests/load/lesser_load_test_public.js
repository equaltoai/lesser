import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');

// Test configuration for public endpoints only
export let options = {
  // Cloud options for Grafana k6 Cloud
  ext: {
    loadimpact: {
      projectID: __ENV.K6_PROJECT_ID,
      name: 'Lesser Public Endpoints Load Test',
      distribution: {
        'amazon:us:ashburn': { loadZone: 'amazon:us:ashburn', percent: 100 }
      }
    }
  },
  
  stages: [
    { duration: '30s', target: 10 },    // Ramp up to 10 users
    { duration: '1m', target: 50 },     // Ramp up to 50 users
    { duration: '2m', target: 100 },    // Stay at 100 users
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95% of requests must complete below 500ms
    errors: ['rate<0.1'],             // Error rate must be below 10%
  },
};

const BASE_URL = __ENV.LESSER_URL || 'https://lesser.example.com';

// Test scenarios - public endpoints only
export default function () {
  const scenario = Math.random();
  
  if (scenario < 0.25) {
    // 25% - Instance info
    testInstanceInfo();
  } else if (scenario < 0.5) {
    // 25% - Public timeline
    testPublicTimeline();
  } else if (scenario < 0.75) {
    // 25% - Search (public)
    testPublicSearch();
  } else {
    // 25% - NodeInfo
    testNodeInfo();
  }
  
  sleep(1);
}

function testInstanceInfo() {
  const response = http.get(`${BASE_URL}/api/v1/instance`);
  
  const success = check(response, {
    'instance info status is 200': (r) => r.status === 200,
    'instance info has domain': (r) => r.json().domain !== undefined,
    'instance info loads fast': (r) => r.timings.duration < 200,
  });
  
  errorRate.add(!success);
}

function testPublicTimeline() {
  const response = http.get(`${BASE_URL}/api/v1/timelines/public?local=true&limit=20`);
  
  const success = check(response, {
    'public timeline status is 200': (r) => r.status === 200,
    'public timeline returns array': (r) => Array.isArray(r.json()),
    'public timeline loads reasonably fast': (r) => r.timings.duration < 500,
  });
  
  errorRate.add(!success);
}

function testPublicSearch() {
  const queries = ['test', 'lesser', 'activitypub', 'federation'];
  const query = queries[Math.floor(Math.random() * queries.length)];
  
  const response = http.get(`${BASE_URL}/api/v2/search?q=${query}&resolve=false&limit=10`);
  
  const success = check(response, {
    'search status is 200': (r) => r.status === 200,
    'search returns results object': (r) => typeof r.json() === 'object',
  });
  
  errorRate.add(!success);
}

function testNodeInfo() {
  const response = http.get(`${BASE_URL}/.well-known/nodeinfo`);
  
  const success = check(response, {
    'nodeinfo status is 200': (r) => r.status === 200,
    'nodeinfo has links': (r) => r.json().links !== undefined,
  });
  
  errorRate.add(!success);
}

// Lifecycle hooks
export function setup() {
  // Verify the instance is reachable
  const response = http.get(`${BASE_URL}/api/v1/instance`);
  
  if (response.status !== 200) {
    throw new Error(`Instance not reachable: ${response.status}`);
  }
  
  console.log(`Testing instance: ${response.json().domain}`);
  console.log(`Version: ${response.json().version}`);
  console.log(`\nNOTE: This test only checks PUBLIC endpoints.`);
  console.log(`For authenticated endpoint testing, use the full test with a valid access token.\n`);
  
  return { startTime: Date.now() };
}

export function teardown(data) {
  const duration = Date.now() - data.startTime;
  console.log(`Test completed in ${duration}ms`);
}

// Custom summary
export function handleSummary(data) {
  return {
    'stdout': textSummary(data),
  };
}

function textSummary(data) {
  return `
Lesser Public Endpoints Load Test Results
=========================================
Total Requests: ${data.metrics.http_reqs.values.count}
Average RPS: ${data.metrics.http_reqs.values.rate.toFixed(2)}
Average Duration: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms
P95 Duration: ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms
Error Rate: ${(data.metrics.errors.values.rate * 100).toFixed(2)}%
Total Data: ${(data.metrics.data_received.values.count / 1024 / 1024).toFixed(2)} MB

This test only covers PUBLIC endpoints. For full testing including:
- Home timeline
- Notifications  
- Creating posts
- User-specific features

You need to provide an access token via LESSER_TOKEN environment variable.
`;
} 