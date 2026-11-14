import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { connectSourceVideo } from './sourceVideo';

class FakeBuffer extends EventTarget {
  updating = false;
  buffered = { length: 0 };
  appendBuffer = vi.fn(() => {
    this.updating = true;
  });
  remove = vi.fn();
}
class FakeMediaSource extends EventTarget {
  static isTypeSupported = vi.fn(() => true);
  static latest: FakeMediaSource;
  readyState = 'closed';
  buffer = new FakeBuffer();
  addSourceBuffer = vi.fn(() => this.buffer);
  constructor() {
    super();
    FakeMediaSource.latest = this;
  }
  open() {
    this.readyState = 'open';
    this.dispatchEvent(new Event('sourceopen'));
  }
}
class FakeSocket {
  static OPEN = 1;
  static latest: FakeSocket;
  readyState = 0;
  binaryType = '';
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  send = vi.fn();
  close = vi.fn();
  constructor(public url: URL) {
    FakeSocket.latest = this;
  }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen?.();
  }
  receive(data: unknown) {
    this.onmessage?.({ data });
  }
}

describe('source video lifecycle', () => {
  let video: HTMLVideoElement;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('MediaSource', FakeMediaSource);
    vi.stubGlobal('WebSocket', FakeSocket);
    vi.stubGlobal(
      'URL',
      class extends URL {
        static createObjectURL = vi.fn(() => 'blob:source-video');
        static revokeObjectURL = vi.fn();
      }
    );
    video = document.createElement('video');
    vi.spyOn(video, 'play').mockResolvedValue();
    vi.spyOn(video, 'pause').mockImplementation(() => {});
    vi.spyOn(video, 'load').mockImplementation(() => {});
    FakeMediaSource.isTypeSupported.mockReturnValue(true);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('negotiates video codecs, queues fragments and releases all playback resources', () => {
    const fail = vi.fn();
    const stop = connectSourceVideo(
      video,
      '/api/v2/integrations/frigate/cameras/feeder/source',
      fail
    );
    const socket = FakeSocket.latest;
    const source = FakeMediaSource.latest;
    socket.open();
    expect(socket.send).not.toHaveBeenCalled();
    source.open();
    expect(socket.url.protocol).toBe('ws:');
    expect(socket.url.searchParams.get('codecs')).not.toContain('mp4a');
    expect(socket.send).toHaveBeenCalledOnce();
    socket.receive(JSON.stringify({ type: 'mse', value: 'video/mp4; codecs="avc1.640029"' }));
    const first = new ArrayBuffer(16);
    const next = new ArrayBuffer(24);
    socket.receive(first);
    socket.receive(next);
    expect(source.buffer.appendBuffer).toHaveBeenCalledExactlyOnceWith(first);
    source.buffer.updating = false;
    source.buffer.dispatchEvent(new Event('updateend'));
    expect(source.buffer.appendBuffer).toHaveBeenLastCalledWith(next);
    video.dispatchEvent(new Event('playing'));
    vi.advanceTimersByTime(20_000);
    expect(fail).not.toHaveBeenCalled();
    stop();
    expect(socket.close).toHaveBeenCalledOnce();
    expect(video.hasAttribute('src')).toBe(false);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:source-video');
    expect(socket.onmessage).toBeNull();
  });

  it('fails safely when codecs are unsupported', async () => {
    FakeMediaSource.isTypeSupported.mockReturnValue(false);
    const fail = vi.fn();
    const stop = connectSourceVideo(video, '/source', fail);
    await Promise.resolve();
    expect(fail).toHaveBeenCalledOnce();
    stop();
  });

  it('bounds startup time and buffered data instead of retaining an unusable connection', () => {
    const fail = vi.fn();
    connectSourceVideo(video, '/source', fail);
    vi.advanceTimersByTime(12_000);
    expect(fail).toHaveBeenCalledOnce();
    expect(FakeSocket.latest.close).toHaveBeenCalledOnce();
    const overflow = vi.fn();
    connectSourceVideo(video, '/source', overflow);
    FakeSocket.latest.receive(new ArrayBuffer(8 * 1024 * 1024 + 1));
    expect(overflow).toHaveBeenCalledOnce();
    expect(FakeSocket.latest.close).toHaveBeenCalledOnce();
  });

  it('returns to the preview when the socket fails and does not signal errors during disposal', () => {
    const fail = vi.fn();
    const stop = connectSourceVideo(video, '/source', fail);
    FakeSocket.latest.onerror?.();
    expect(fail).toHaveBeenCalledOnce();
    stop();
    expect(fail).toHaveBeenCalledOnce();
  });
});
