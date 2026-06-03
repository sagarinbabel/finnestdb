#!/usr/bin/env node

const baseURL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const password = process.env.SMOKE_PASSWORD || 'test-pass-123';
const stamp = `${Date.now()}-${Math.random().toString(16).slice(2)}`;

let pass = 0;
let fail = 0;
const results = [];

function record(name, ok, detail = '') {
  if (ok) {
    pass++;
    results.push({ status: 'PASS', name, detail });
  } else {
    fail++;
    results.push({ status: 'FAIL', name, detail });
  }
}

function getSetCookies(res) {
  if (typeof res.headers.getSetCookie === 'function') return res.headers.getSetCookie();
  const raw = res.headers.get('set-cookie');
  return raw ? [raw] : [];
}

function cookieHeader(cookies) {
  return cookies.map(c => c.split(';')[0]).join('; ');
}

async function request(path, opts = {}) {
  const res = await fetch(`${baseURL}${path}`, opts);
  const text = await res.text();
  return { res, text, cookies: getSetCookies(res) };
}

async function register(email) {
  const r = await request('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  if (r.res.status !== 200) {
    throw new Error(`register ${email} status=${r.res.status} body=${r.text}`);
  }
  return r.cookies;
}

async function main() {
  const userEmail = `smoke-${stamp}@example.com`;
  const cookies = await register(userEmail);
  const cookie = cookieHeader(cookies);
  record('register returns session cookie', cookie.includes('session_token='));

  let r = await request('/api/me', { headers: { Cookie: cookie } });
  record('/api/me authenticated', r.res.status === 200 && r.text.includes(userEmail), `status=${r.res.status}`);

  r = await request('/api/admin/users', { headers: { Cookie: cookie } });
  record('non-admin admin API rejected', r.res.status === 403, `status=${r.res.status}`);

  r = await request('/api/parse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: cookie },
    body: JSON.stringify({ lang: 'FI', parser: 'custom', text: 'Kissa juoksee nopeasti.' }),
  });
  const parse = JSON.parse(r.text);
  record('authenticated parse succeeds and remains ephemeral', r.res.status === 200 && !('parse_id' in parse) && parse.words?.length > 0, `status=${r.res.status}`);

  r = await request('/api/parse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ lang: 'ET', parser: 'custom', text: 'Koer jookseb kiiresti.' }),
  });
  record('anonymous parse allowed as documented', r.res.status === 200, `status=${r.res.status}`);

  r = await request('/api/lemma-state', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: 'https://evil.example', Cookie: cookie },
    body: JSON.stringify({ lang: 'FI', lemma: 'kissa', pos: 'NOUN', status: 'known' }),
  });
  record('foreign-origin authenticated mutation rejected', r.res.status === 403, `status=${r.res.status}`);

  r = await request('/api/lemma-state', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: baseURL, Cookie: cookie },
    body: JSON.stringify({ lang: 'FI', lemma: 'kissa', pos: 'NOUN', status: 'known' }),
  });
  record('same-origin authenticated mutation allowed', r.res.status === 200, `status=${r.res.status}`);

  r = await request('/api/parse/feedback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: cookie },
    body: JSON.stringify({
      lang: 'FI',
      parser: 'custom',
      source_text: 'Kissa juoksee nopeasti.',
      total_tokens: 3,
      unique_lemma_count: 3,
      surface: 'Kissa',
      occurrence: 0,
      original_lemma: 'kissa',
      original_pos: 'NOUN',
      proposed_lemma: 'kissa',
      proposed_pos: 'PROPN',
    }),
  });
  record('feedback creates retained parse context', r.res.status === 200, `status=${r.res.status}`);

  r = await request('/api/parse/sessions', { headers: { Cookie: cookie } });
  const history = JSON.parse(r.text);
  const sessionID = history.sessions?.[0]?.id;
  record('parse history lists retained session', r.res.status === 200 && sessionID > 0 && history.sessions[0].source_preview.includes('Kissa'), `status=${r.res.status}`);

  r = await request(`/api/parse/sessions/${sessionID}`, { method: 'DELETE', headers: { Cookie: cookie } });
  record('parse history per-row delete works', r.res.status === 200, `status=${r.res.status}`);

  r = await request('/api/parse/sessions', { headers: { Cookie: cookie } });
  const afterDelete = JSON.parse(r.text);
  record('parse history empty after delete', r.res.status === 200 && afterDelete.sessions.length === 0, `status=${r.res.status}`);

  const big = 'x'.repeat(4 * 1024 * 1024 + 1024);
  r = await request('/api/parse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: cookie },
    body: JSON.stringify({ lang: 'FI', parser: 'custom', text: big }),
  });
  record('oversized parse request rejected', r.res.status === 400 || r.res.status === 413, `status=${r.res.status}`);

  let hit429 = false;
  for (let i = 0; i < 12; i++) {
    r = await request('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: `missing-${stamp}-${i}@example.com`, password }),
    });
    if (r.res.status === 429) {
      hit429 = true;
      break;
    }
  }
  record('login rate limit triggers', hit429);

  const deleteCookies = await register(`delete-smoke-${stamp}@example.com`);
  const deleteCookie = cookieHeader(deleteCookies);
  r = await request('/api/me', { method: 'DELETE', headers: { Cookie: deleteCookie } });
  record('account deletion endpoint succeeds', r.res.status === 200, `status=${r.res.status}`);
  r = await request('/api/me', { headers: { Cookie: deleteCookie } });
  record('deleted account session no longer authenticates', r.res.status === 200 && r.text.includes('"authenticated":false'), `status=${r.res.status}`);
}

try {
  await main();
} catch (err) {
  record('smoke harness exception', false, err.stack || String(err));
}

for (const row of results) {
  console.log(`${row.status}\t${row.name}${row.detail ? `\t${row.detail}` : ''}`);
}
console.log(`SUMMARY pass=${pass} fail=${fail} baseURL=${baseURL}`);
process.exit(fail === 0 ? 0 : 1);
