// The simulated user (METHODOLOGY.md, "The simulated user"). Held fixed: any
// change to this file is a held-fixed change and invalidates earlier runs.
//
// Each virtual user is one seeded account. It logs in once during the
// unmeasured ramp-up, then loops: feed → think → open a post → think →
// 10% like → 15% create → think. Think time is 5-10 s, three per loop, which
// lands on ~0.1 requests/second per user. A small share of requests go wrong
// on purpose (METHODOLOGY, "Unhappy paths"); each declares the status it
// expects, so only a mismatch counts as a failure.
//
// Only the steady window is judged: every request is tagged phase=rampup,
// settle or steady, and the thresholds read phase:steady only.
//
// Env: BASE_URL, USERS, LOGIN_RATE (logins/s during ramp-up), SETTLE, STEADY
// (seconds), RUN_SEED.

import http from 'k6/http';
import { check, randomSeed, sleep } from 'k6';
import encoding from 'k6/encoding';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const USERS = int('USERS', 50);
const LOGIN_RATE = int('LOGIN_RATE', 25);
const SETTLE = int('SETTLE', 30);
const STEADY = int('STEADY', 120);
const RUN_SEED = int('RUN_SEED', 1);

const SEEDED_USERS = 50000; // seed.Users
const PASSWORD = 'stresslab-password'; // seed.Password: synthetic accounts
const MISSING_POST = 900000000; // above any seeded or created post ID
const RAMP = Math.ceil(USERS / LOGIN_RATE);

export const options = {
  scenarios: {
    users: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: `${RAMP}s`, target: USERS },
        { duration: `${SETTLE + STEADY}s`, target: USERS },
      ],
      gracefulRampDown: '0s',
      gracefulStop: '15s',
    },
  },
  thresholds: {
    // The SLOs (SPEC, "Service level objectives"), steady window only.
    'http_req_duration{phase:steady}': ['p(95)<500', 'p(99)<1000'],
    'http_req_failed{phase:steady}': ['rate<0.01'],
    'checks{phase:steady}': ['rate>0.99'],
    // Not limits — they make the steady-window counts show up in the summary.
    'http_reqs{phase:steady}': ['count>=0'],
    'http_reqs{phase:rampup}': ['count>=0'],
  },
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
};

let seeded = false;
let token = null;

export default function () {
  if (!seeded) {
    // Seed per user, inside user code: calling randomSeed at the top of the
    // script doesn't make k6 v2 runs reproducible (checked 2026-10-03).
    randomSeed(RUN_SEED * 100003 + exec.vu.idInTest);
    seeded = true;
  }
  if (!token) login();

  const feed = authed('GET', '/v1/feed', null, 200, 'GET /v1/feed');
  const posts = feed.status === 200 ? feed.json('posts') : [];
  check(feed, { 'feed has 20 posts': () => posts.length === 20 }, { phase: phase() });
  think();

  let postId = null;
  if (Math.random() < 0.02 || posts.length === 0) {
    const r = authed('GET', `/v1/posts/${MISSING_POST + randInt(1000)}`, null, 404, 'GET /v1/posts/{id}');
    check(r, { 'missing post is 404': (x) => x.status === 404 }, { phase: phase() });
  } else {
    postId = posts[randInt(posts.length)].id;
    const r = authed('GET', `/v1/posts/${postId}`, null, 200, 'GET /v1/posts/{id}');
    check(r, { 'post matches': (x) => x.status === 200 && x.json('id') === postId }, { phase: phase() });
  }
  think();

  if (postId !== null && Math.random() < 0.1) {
    authed('POST', `/v1/posts/${postId}/like`, null, 204, 'POST /v1/posts/{id}/like');
  }
  if (Math.random() < 0.15) {
    if (Math.random() < 0.01) {
      authed('POST', '/v1/posts', JSON.stringify({ body: '' }), 400, 'POST /v1/posts');
    } else {
      const r = authed('POST', '/v1/posts', JSON.stringify({ body: postBody() }), 201, 'POST /v1/posts');
      check(r, { 'post created': (x) => x.status === 201 }, { phase: phase() });
    }
  }
  think();
}

// login: 1% of logins start with a wrong password, 0.5% with an unknown user,
// then log in properly.
function login() {
  const account = `user_${((exec.vu.idInTest - 1) % SEEDED_USERS) + 1}`;
  const r = Math.random();
  if (r < 0.01) {
    basicLogin(account, 'wrong-password', 401);
  } else if (r < 0.015) {
    basicLogin(`no_such_user_${exec.vu.idInTest}`, PASSWORD, 401);
  }
  const res = basicLogin(account, PASSWORD, 200);
  token = res.status === 200 ? res.json('token') : null;
}

function basicLogin(user, password, expect) {
  return send('POST', '/v1/login', null,
    { Authorization: `Basic ${encoding.b64encode(`${user}:${password}`)}` },
    expect, 'POST /v1/login');
}

// authed sends with the user's token. 0.5% of requests are preceded by one
// carrying a garbage token. An unexpected 401 means the token expired: log in
// again and retry once.
function authed(method, path, body, expect, name) {
  if (Math.random() < 0.005) {
    send(method, path, body, { Authorization: 'Bearer garbage' }, 401, name);
  }
  let res = send(method, path, body, bearer(), expect, name);
  if (res.status === 401 && expect !== 401) {
    login();
    res = send(method, path, body, bearer(), expect, name);
  }
  return res;
}

function send(method, path, body, headers, expect, name) {
  const h = Object.assign({}, headers);
  if (body !== null) h['Content-Type'] = 'application/json';
  return http.request(method, `${BASE_URL}${path}`, body, {
    headers: h,
    tags: { name, phase: phase() },
    responseCallback: http.expectedStatuses(expect),
  });
}

function bearer() {
  return { Authorization: `Bearer ${token}` };
}

// phase reports which part of the run this moment belongs to.
function phase() {
  const elapsed = (Date.now() - exec.scenario.startTime) / 1000;
  if (elapsed < RAMP) return 'rampup';
  if (elapsed < RAMP + SETTLE) return 'settle';
  return 'steady';
}

function think() {
  sleep(5 + Math.random() * 5);
}

const WORDS = ('load limit cache query index pool replica queue feed post latency budget ' +
  'throughput goodput tail server deploy coffee the a to and of is it that for on with')
  .split(' ');

// postBody is 20-200 characters from a fixed word list.
function postBody() {
  const target = 20 + randInt(181);
  let s = WORDS[randInt(WORDS.length)];
  while (s.length < target) s += ' ' + WORDS[randInt(WORDS.length)];
  return s.slice(0, target).trim() || 'post';
}

function randInt(n) {
  return Math.floor(Math.random() * n);
}

function int(name, fallback) {
  const v = __ENV[name];
  return v === undefined || v === '' ? fallback : parseInt(v, 10);
}
