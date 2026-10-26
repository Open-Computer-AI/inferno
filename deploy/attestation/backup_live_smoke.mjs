import {readFileSync} from 'node:fs';
import {pathToFileURL} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';

const input = JSON.parse(readFileSync(0, 'utf8'));
const worker = input.workerDist;
const {InfernoLiveTransport, createSessionContextAppends} = await import(pathToFileURL(`${worker}/inferno-live-transport.js`));
const {createWrtcInfernoPeerConnectionFactory} = await import(pathToFileURL(`${worker}/wrtc-inferno-peer.js`));
const {createWsInfernoSidebandFactory} = await import(pathToFileURL(`${worker}/ws-inferno-sideband.js`));
const started = performance.now();
const elapsed = () => Math.round((performance.now() - started) * 1000) / 1000;
const counts = {};
const protocol = {};
const result = {scenario: input.scenario, success: false, nonzeroAudioFrames: 0, audibleFramesAfterAppend: 0, peakAbs: 0};
let session;
let appendAt;
const originalFetch = globalThis.fetch;
const transport = new InfernoLiveTransport({
  infernoUrl: 'http://127.0.0.1:19480/v1/live',
  authorizationToken: input.key,
  peerConnectionFactory: createWrtcInfernoPeerConnectionFactory(),
  sidebandFactory: createWsInfernoSidebandFactory(input.key),
  fetch: async (...args) => {
    const start = performance.now();
    const response = await originalFetch(...args);
    result.http_status = response.status;
    result.http_create_headers_ms = Math.round((performance.now() - start) * 1000) / 1000;
    return response;
  },
  logger: message => {
    if (message === 'Inferno transport: SDP accepted by Inferno') result.sdp_accepted = true;
    if (message === 'Inferno transport: sideband connected') result.sideband_ready_ms = elapsed();
  },
  protocolObserver: event => {
    const kind = /^[a-zA-Z0-9_.-]{1,80}$/.test(event.eventType) ? event.eventType : 'other';
    const label = `${event.channel}:${event.direction}:${kind}`;
    protocol[label] = (protocol[label] ?? 0) + 1;
  },
});
const abort = new AbortController();
let timedOut = false;
let rejectTimeout;
const timeout = new Promise((_, reject) => { rejectTimeout = reject; });
const watchdog = setTimeout(() => {
  timedOut = true;
  abort.abort();
  rejectTimeout(new Error('overall_timeout'));
}, 90_000);
const bounded = async (operation, ms) => {
  let timer;
  try {
    return await Promise.race([operation, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('cleanup_timeout')), ms);
    })]);
  } finally { clearTimeout(timer); }
};
try {
  await Promise.race([(async () => {
  session = await transport.createSession({
    model: 'gpt-live-1-codex', voice: 'maple',
    instructions: 'You are an isolated voice connection test. Respond briefly in English.',
  }, AbortSignal.any([abort.signal, AbortSignal.timeout(45_000)]));
  result.transport_ready_ms = elapsed();
  console.log(JSON.stringify({stage: 'session_ready', scenario: input.scenario, ms: result.transport_ready_ms}));
  session.onEvent(event => {
    counts[event.type] = (counts[event.type] ?? 0) + 1;
    if (event.type === 'audio_output' && event.frame.data.some(value => value !== 0)) {
      result.nonzeroAudioFrames++;
      let energy = 0;
      for (const value of event.frame.data) {
        energy += value * value;
        result.peakAbs = Math.max(result.peakAbs, Math.abs(value));
      }
      const audible = Math.sqrt(energy / event.frame.data.length) >= 100;
      if (appendAt !== undefined && audible) result.audibleFramesAfterAppend++;
      if (appendAt !== undefined && audible && result.first_audio_after_append_ms === undefined) {
        result.first_audio_after_append_ms = Math.round((performance.now() - appendAt) * 1000) / 1000;
      }
    }
  });
  for (let i = 0; i < 100; i++) {
    if (timedOut) throw new Error('overall_timeout');
    await session.inputAudio({data: new Int16Array(480), sampleRate: 48_000, channels: 1, samplesPerChannel: 480});
    await delay(10);
  }
  appendAt = performance.now();
  for (const event of createSessionContextAppends('Say: the attestation backup test is working.', 'speakable')) {
    await session.sendControl(event);
  }
  const deadline = performance.now() + 25_000;
  while (performance.now() < deadline && result.audibleFramesAfterAppend < 20 && !counts.error) await delay(100);
  if (timedOut) throw new Error('overall_timeout');
  result.success = Boolean(result.sdp_accepted && result.sideband_ready_ms && counts.session_started && result.audibleFramesAfterAppend >= 20 && !counts.error);
  })(), timeout]);
} catch (error) {
  result.error_class = error?.name ?? 'Error';
  const match = /^Inferno session request failed \(status (\d{3})\)$/.exec(error?.message ?? '');
  if (match) result.http_status = Number(match[1]);
  if (input.expectUnavailable && result.http_status === 503) {
    result.fail_closed = true;
    result.success = true;
  }
} finally {
  abort.abort();
  clearTimeout(watchdog);
  if (session) {
    await bounded(session.sendControl({type: 'session.close'}), 1000).catch(() => {});
    result.close_control_sent = true;
    const deadline = performance.now() + 5000;
    while (performance.now() < deadline && !counts.session_closed) await delay(100);
    await bounded(session.close(), 2000).then(() => { result.transports_closed = true; }, () => { result.cleanup_timeout = true; result.success = false; });
    result.close_ack = Boolean(counts.session_closed);
  }
  clearTimeout(watchdog);
  result.counts = counts;
  result.protocol = protocol;
  result.total_ms = elapsed();
  console.log(JSON.stringify(result));
  process.exit(result.success ? 0 : 1);
}
