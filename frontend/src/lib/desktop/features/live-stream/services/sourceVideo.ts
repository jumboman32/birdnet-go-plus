// go2rtc negotiates browser-supported codecs, then sends fragmented MP4 over WS.
// Only video codecs are offered: this player does not create an audio pipeline.
const VIDEO_CODECS = ['avc1.640029', 'avc1.64002A', 'avc1.640033', 'hvc1.1.6.L153.B0'];
const MAX_QUEUED_BYTES = 8 * 1024 * 1024;
const STARTUP_TIMEOUT_MS = 12_000;
const BUFFER_RETENTION_SECONDS = 10;
const BUFFER_PRUNE_SECONDS = 5;
const MAX_LIVE_DELAY_SECONDS = 2;
const LIVE_EDGE_MARGIN_SECONDS = 0.3;

/** Connect source video without resizing, frame-rate conversion or audio. */
export function connectSourceVideo(
  video: HTMLVideoElement,
  path: string,
  onError: () => void
): () => void {
  let disposed = false;
  let socket: WebSocket | undefined;
  let source: MediaSource | undefined;
  let buffer: SourceBuffer | undefined;
  let objectURL: string | undefined;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let negotiated = false;
  const pending: ArrayBuffer[] = [];
  let queuedBytes = 0;

  function dispose() {
    if (disposed) return;
    disposed = true;
    clearTimeout(timeout);
    video.removeEventListener('playing', playing);
    video.removeEventListener('error', fail);
    buffer?.removeEventListener('updateend', pump);
    source?.removeEventListener('sourceopen', negotiate);
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      socket.close();
    }
    pending.length = 0;
    video.pause();
    video.removeAttribute('src');
    video.load();
    if (objectURL) URL.revokeObjectURL(objectURL);
  }
  function fail() {
    if (disposed) return;
    dispose();
    onError();
  }
  function playing() {
    clearTimeout(timeout);
  }
  function negotiate() {
    if (
      disposed ||
      negotiated ||
      source?.readyState !== 'open' ||
      socket?.readyState !== WebSocket.OPEN
    )
      return;
    negotiated = true;
    socket.send(JSON.stringify({ type: 'mse', value: codecs.join(',') }));
  }
  function pump() {
    if (disposed || !buffer || buffer.updating || source?.readyState !== 'open') return;
    try {
      if (buffer.buffered.length) {
        const end = buffer.buffered.end(buffer.buffered.length - 1);
        const start = buffer.buffered.start(0);
        if (end - video.currentTime > MAX_LIVE_DELAY_SECONDS) {
          video.currentTime = Math.max(start, end - LIVE_EDGE_MARGIN_SECONDS);
        }
        const cutoff = Math.min(video.currentTime, end) - BUFFER_RETENTION_SECONDS;
        if (cutoff - start > BUFFER_PRUNE_SECONDS) {
          buffer.remove(start, cutoff);
          return;
        }
      }
      const next = pending.shift();
      if (next) {
        queuedBytes -= next.byteLength;
        buffer.appendBuffer(next);
      }
    } catch {
      fail();
    }
  }

  const codecs =
    typeof MediaSource === 'undefined'
      ? []
      : VIDEO_CODECS.filter(codec => MediaSource.isTypeSupported(`video/mp4; codecs="${codec}"`));
  if (!codecs.length) {
    queueMicrotask(() => {
      if (!disposed) fail();
    });
    return dispose;
  }
  try {
    const url = new URL(path, window.location.href);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    url.searchParams.set('codecs', codecs.join(','));
    source = new MediaSource();
    objectURL = URL.createObjectURL(source);
    source.addEventListener('sourceopen', negotiate);
    video.addEventListener('playing', playing);
    video.addEventListener('error', fail);
    socket = new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    socket.onopen = negotiate;
    socket.onerror = fail;
    socket.onclose = fail;
    socket.onmessage = event => {
      if (disposed) return;
      if (typeof event.data === 'string') {
        try {
          const message: unknown = JSON.parse(event.data);
          if (
            typeof message !== 'object' ||
            message === null ||
            !('type' in message) ||
            message.type !== 'mse' ||
            !('value' in message) ||
            typeof message.value !== 'string' ||
            buffer ||
            !MediaSource.isTypeSupported(message.value)
          ) {
            fail();
            return;
          }
          buffer = source?.addSourceBuffer(message.value);
          buffer?.addEventListener('updateend', pump);
          pump();
        } catch {
          fail();
        }
      } else if (event.data instanceof ArrayBuffer) {
        queuedBytes += event.data.byteLength;
        if (queuedBytes > MAX_QUEUED_BYTES) {
          fail();
          return;
        }
        pending.push(event.data);
        pump();
      } else {
        fail();
      }
    };
    video.src = objectURL;
    video.muted = true;
    timeout = setTimeout(fail, STARTUP_TIMEOUT_MS);
    void video.play().catch(fail);
  } catch {
    fail();
  }
  return dispose;
}
