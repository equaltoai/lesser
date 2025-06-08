import http from 'k6/http';
import { check, sleep } from 'k6';

// Simple test configuration
export const options = {
  stages: [
    { duration: '30s', target: 5 },   // Ramp up to 5 users
    { duration: '1m', target: 10 },   // Stay at 10 users
    { duration: '30s', target: 0 },   // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95% of requests must complete below 500ms
  },
};

export default function () {
  // Test against httpbin.io for demo purposes
  const response = http.get('https://httpbin.org/delay/0.3');
  
  check(response, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
  
  sleep(1);
}

export function handleSummary(data) {
  console.log('\n=== Test Summary ===');
  console.log(`Total Requests: ${data.metrics.http_reqs.values.count}`);
  console.log(`Average RPS: ${data.metrics.http_reqs.values.rate.toFixed(2)}`);
  console.log(`Average Duration: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms`);
  console.log(`P95 Duration: ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms`);
  
  return {};
} 