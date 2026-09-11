import { describe, expect, it, vi } from 'vitest';
import { createConsoleServer } from './server.mjs';
import http from 'node:http';

const listen = (server) => new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve(server.address().port)));
const call = (port, options, body = '') => new Promise((resolve, reject) => { const req = http.request({host: '127.0.0.1', port, ...options}, (res) => { let data = ''; res.on('data', (x) => { data += x; }); res.on('end', () => resolve({status: res.statusCode, headers: res.headers, body: data})); }); req.on('error', reject); if (body) req.write(body); req.end(); });

describe('GPT Live broker', () => {
  it('sends the exact authenticated contract and projects the response', async () => {
    const upstream = vi.fn().mockResolvedValue({ok: true, json: async () => ({session: {id: 'opaque'}, transport: {sdp: 'answer'}})});
    const server = createConsoleServer({fetchImpl: upstream}); const port = await listen(server);
    const result = await call(port, {method: 'POST', path: '/api/session', headers: {origin: `http://127.0.0.1:${port}`, 'content-type': 'application/json'}}, JSON.stringify({apiKey: 'sentinel-key', sdp: 'offer'}));
    expect(result.status).toBe(201); expect(result.headers['cache-control']).toBe('no-store'); expect(JSON.parse(result.body)).toEqual({session: {id: 'opaque'}, transport: {sdp: 'answer'}});
    expect(upstream).toHaveBeenCalledWith('https://api.openai.com/v1/live/sessions', expect.objectContaining({method: 'POST', headers: {authorization: 'Bearer sentinel-key', 'content-type': 'application/json'}, body: JSON.stringify({session: {model: 'gpt-live-1', delegation: {type: 'client'}}, transport: {type: 'webrtc', sdp: 'offer'}})}));
    server.close();
  });
  it('rejects foreign origins, invalid input, and oversized bodies without upstream calls', async () => {
    const upstream = vi.fn(); const server = createConsoleServer({fetchImpl: upstream}); const port = await listen(server);
    expect((await call(port, {method: 'POST', path: '/api/session', headers: {origin: 'https://evil.example'}}, '{}')).status).toBe(403);
    expect((await call(port, {method: 'POST', path: '/api/session'}, JSON.stringify({apiKey: '', sdp: ''}))).status).toBe(400);
    expect((await call(port, {method: 'POST', path: '/api/session'}, 'x'.repeat(65537))).status).toBe(413);
    expect(upstream).not.toHaveBeenCalled(); server.close();
  });
  it('sanitizes upstream failures and serves only allowlisted assets', async () => {
    const server = createConsoleServer({fetchImpl: vi.fn().mockResolvedValue({ok: false, json: async () => ({secret: 'sentinel-key'})}), logger: vi.fn()}); const port = await listen(server);
    const failure = await call(port, {method: 'POST', path: '/api/session'}, JSON.stringify({apiKey: 'sentinel-key', sdp: 'offer'}));
    expect(failure.status).toBe(502); expect(failure.body).not.toContain('sentinel-key'); expect((await call(port, {method: 'GET', path: '/app.js'})).status).toBe(200); expect((await call(port, {method: 'GET', path: '/secret'})).status).toBe(404); server.close();
  });
});
