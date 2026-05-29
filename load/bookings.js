import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000'],
    checks: ['rate>0.99'],
  },
};

const baseUrl = __ENV.BOOKING_API_URL || 'http://localhost:8000';

export default function () {
  const search = http.get(`${baseUrl}/flights?origin=SVO&destination=LED&date=2026-04-01`);
  check(search, {
    'search status is 200': (r) => r.status === 200,
    'search returns flights': (r) => Array.isArray(r.json('flights')) && r.json('flights').length > 0,
  });

  const flight = http.get(`${baseUrl}/flights/1`);
  check(flight, {
    'flight status is 200': (r) => r.status === 200,
    'flight has available seats field': (r) => typeof r.json('available_seats') === 'number',
  });

  sleep(1);
}

export function handleSummary(data) {
  return {
    'load-results/summary.json': JSON.stringify(data, null, 2),
    stdout: JSON.stringify(data.metrics, null, 2),
  };
}
