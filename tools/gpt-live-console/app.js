const ui = {
  key: document.querySelector('#api-key'), start: document.querySelector('#start'), stop: document.querySelector('#stop'),
  status: document.querySelector('#status'), audio: document.querySelector('#remote-audio'),
};
let peer; let channel; let stream; let startup; let stopTimer;
const setStatus = (text) => { ui.status.textContent = text; };
const cleanup = () => {
  if (stopTimer) { clearTimeout(stopTimer); stopTimer = undefined; }
  if (channel) { channel.close(); channel = undefined; }
  if (peer) { peer.close(); peer = undefined; }
  if (stream) { stream.getTracks().forEach((track) => track.stop()); stream = undefined; }
  ui.audio.srcObject = null; ui.key.value = ''; ui.start.disabled = false; ui.stop.disabled = true; startup?.abort(); startup = undefined;
};
const fail = () => { setStatus('Error: session could not be started'); cleanup(); };
async function start() {
  cleanup(); setStatus('Connecting'); ui.start.disabled = true; ui.stop.disabled = false;
  startup = new AbortController();
  try {
    stream = await navigator.mediaDevices.getUserMedia({audio: true});
    peer = new RTCPeerConnection(); stream.getTracks().forEach((track) => peer.addTrack(track, stream));
    channel = peer.createDataChannel('oai-events');
    channel.addEventListener('message', (event) => {
      try { const msg = JSON.parse(event.data); if (msg.type === 'session.started') setStatus('Connected'); if (msg.type === 'session.closed') { cleanup(); setStatus('Stopped'); } if (msg.type === 'error') fail(); } catch { /* ignore malformed events */ }
    });
    channel.addEventListener('close', () => { if (ui.stop.disabled === false) cleanup(); });
    peer.addEventListener('track', (event) => { ui.audio.srcObject = event.streams[0]; });
    const offer = await peer.createOffer(); await peer.setLocalDescription(offer);
    await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(new Error('ICE timeout')), 10000); if (peer.iceGatheringState === 'complete') { clearTimeout(timer); resolve(); } else peer.addEventListener('icegatheringstatechange', () => { if (peer.iceGatheringState === 'complete') { clearTimeout(timer); resolve(); } }); });
    const apiKey = ui.key.value; ui.key.value = '';
    const response = await fetch('/api/session', {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify({apiKey, sdp: peer.localDescription.sdp}), signal: startup.signal});
    if (!response.ok) throw new Error('session failed'); const result = await response.json();
    if (!result?.transport?.sdp) throw new Error('invalid answer'); await peer.setRemoteDescription({type: 'answer', sdp: result.transport.sdp});
  } catch { fail(); }
}
function stop() {
  if (!peer && !channel && !stream) return; setStatus('Stopping'); ui.stop.disabled = true;
  if (channel?.readyState === 'open') channel.send(JSON.stringify({type: 'session.close'}));
  stopTimer = setTimeout(() => { cleanup(); setStatus('Stopped (finalization incomplete)'); }, 15000);
}
ui.start.addEventListener('click', start); ui.stop.addEventListener('click', stop); window.addEventListener('beforeunload', cleanup);
