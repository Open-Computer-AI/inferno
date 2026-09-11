import { beforeEach, describe, expect, it, vi } from 'vitest';

const mediaTracks = [{stop: vi.fn()}];
const channel = {readyState: 'open', listeners: {}, addEventListener: vi.fn((name, fn) => { channel.listeners[name] = fn; }), send: vi.fn(), close: vi.fn()};
const peer = {iceGatheringState: 'complete', listeners: {}, addTrack: vi.fn(), createDataChannel: vi.fn(() => channel), createOffer: vi.fn(async () => ({type: 'offer', sdp: 'offer'})), setLocalDescription: vi.fn(async () => {}), setRemoteDescription: vi.fn(async () => {}), addEventListener: vi.fn((name, fn) => { peer.listeners[name] = fn; }), close: vi.fn()};

describe('GPT Live app', () => {
  beforeEach(() => { vi.resetModules(); vi.restoreAllMocks(); document.body.innerHTML = '<input id="api-key"><button id="start"></button><button id="stop"></button><p id="status"></p><audio id="remote-audio"></audio>'; Object.assign(globalThis, {RTCPeerConnection: vi.fn(() => peer), fetch: vi.fn(async () => ({ok: true, json: async () => ({transport: {sdp: 'answer'}})}))}); navigator.mediaDevices = {getUserMedia: vi.fn(async () => ({getTracks: () => mediaTracks}))}; });
  it('orders media, channel, offer, and broker request while clearing key', async () => {
    document.querySelector('#api-key').value = 'sentinel-key'; await import('./app.js'); document.querySelector('#start').click(); await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(peer.addTrack).toHaveBeenCalled(); expect(peer.createDataChannel).toHaveBeenCalledWith('oai-events'); expect(peer.createOffer).toHaveBeenCalled(); expect(fetch).toHaveBeenCalledWith('/api/session', expect.objectContaining({method: 'POST'})); expect(document.querySelector('#api-key').value).toBe('');
  });
  it('stops the session and local tracks through cleanup', async () => {
    document.querySelector('#api-key').value = 'sentinel-key'; await import('./app.js'); document.querySelector('#start').click(); await vi.waitFor(() => expect(fetch).toHaveBeenCalled()); document.querySelector('#stop').click(); expect(channel.send).toHaveBeenCalledWith(JSON.stringify({type: 'session.close'})); channel.listeners.message({data: JSON.stringify({type: 'session.closed'})}); expect(mediaTracks[0].stop).toHaveBeenCalled(); expect(peer.close).toHaveBeenCalled();
  });
});
