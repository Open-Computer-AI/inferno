import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const ROOT = path.dirname(fileURLToPath(import.meta.url));
const ALLOWED = new Map([
  ['/', 'index.html'], ['/index.html', 'index.html'], ['/app.js', 'app.js'], ['/styles.css', 'styles.css'],
]);
const MAX_BODY = 64 * 1024;
const LIVE_URL = 'https://api.openai.com/v1/live/sessions';

const json = (res, status, value) => {
  const body = JSON.stringify(value);
  res.writeHead(status, {'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store'});
  res.end(body);
};

async function requestBody(req) {
  let size = 0; const chunks = [];
  for await (const chunk of req) {
    size += chunk.length;
    if (size > MAX_BODY) throw Object.assign(new Error('request too large'), { code: 'TOO_LARGE' });
    chunks.push(chunk);
  }
  try { return JSON.parse(Buffer.concat(chunks).toString('utf8')); }
  catch { throw Object.assign(new Error('invalid JSON'), { code: 'BAD_JSON' }); }
}

export function createConsoleServer({ fetchImpl = fetch, logger = () => {}, staticRoot = ROOT } = {}) {
  const server = createServer(async (req, res) => {
    try {
      if (req.method === 'POST' && req.url === '/api/session') {
        if (req.headers.origin && !/^http:\/\/127\.0\.0\.1(?::\d+)?$/.test(req.headers.origin)) return json(res, 403, {error: 'origin not allowed'});
        const input = await requestBody(req);
        if (!input || typeof input.apiKey !== 'string' || !input.apiKey.trim() || typeof input.sdp !== 'string' || !input.sdp.trim()) return json(res, 400, {error: 'apiKey and sdp are required'});
        const upstream = await fetchImpl(LIVE_URL, {method: 'POST', headers: {'authorization': `Bearer ${input.apiKey}`, 'content-type': 'application/json'}, body: JSON.stringify({session: {model: 'gpt-live-1', delegation: {type: 'client'}}, transport: {type: 'webrtc', sdp: input.sdp}})});
        if (!upstream.ok) return json(res, 502, {error: 'upstream session request failed'});
        const result = await upstream.json();
        if (!result?.session?.id || typeof result?.transport?.sdp !== 'string') return json(res, 502, {error: 'invalid upstream response'});
        return json(res, 201, {session: {id: result.session.id}, transport: {sdp: result.transport.sdp}});
      }
      if (req.method === 'GET' && ALLOWED.has(req.url)) {
        const name = ALLOWED.get(req.url); const text = await readFile(path.join(staticRoot, name));
        const type = name.endsWith('.js') ? 'text/javascript' : name.endsWith('.css') ? 'text/css' : 'text/html';
        res.writeHead(200, {'content-type': `${type}; charset=utf-8`, 'cache-control': 'no-store'}); return res.end(text);
      }
      json(res, 404, {error: 'not found'});
    } catch (error) {
      if (error.code === 'TOO_LARGE') return json(res, 413, {error: 'request too large'});
      if (error.code === 'BAD_JSON') return json(res, 400, {error: 'invalid JSON'});
      logger('session request failed'); json(res, 502, {error: 'session request failed'});
    }
  });
  return server;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const server = createConsoleServer({logger: console.error});
  server.listen(0, '127.0.0.1', () => logger(`GPT Live console: http://127.0.0.1:${server.address().port}/`));
}
