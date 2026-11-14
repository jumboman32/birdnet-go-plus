import { describe, it, expect, vi, afterEach } from 'vitest';
import { fireEvent, screen, waitFor, cleanup } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import { api } from '$lib/utils/api';
import CameraLivePage from './CameraLivePage.svelte';

describe('Live cameras', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('shows how to configure a disabled integration', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ enabled: false, cameras: [] })
      .mockResolvedValue({ data: [] });
    renderTyped(CameraLivePage);
    expect(await screen.findByText('cameraLive.disabled')).toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
  });

  it('Accessibility: announces pause and releases the same-origin video stream', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        enabled: true,
        cameras: [{ name: 'feeder', online: true, fps: 4 }],
      })
      .mockResolvedValue({ data: [] });
    renderTyped(CameraLivePage);
    const image = await screen.findByRole('img', { name: 'feeder' });
    expect(image.getAttribute('src')).toContain(
      '/api/v2/integrations/frigate/cameras/feeder/live?session='
    );
    await fireEvent.click(screen.getByRole('button', { name: 'cameraLive.pause' }));
    await waitFor(() => expect(screen.queryByRole('img')).not.toBeInTheDocument());
    expect(
      screen
        .getAllByRole('status')
        .some(element => element.textContent.includes('cameraLive.paused'))
    ).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'cameraLive.resume' }));
    expect(await screen.findByRole('img', { name: 'feeder' })).toBeInTheDocument();
  });

  it('does not request video from an offline camera', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        enabled: true,
        cameras: [{ name: 'feeder', online: false, fps: 0 }],
      })
      .mockResolvedValue({ data: [] });
    renderTyped(CameraLivePage);
    expect(await screen.findByText('cameraLive.offlineHelp')).toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
  });

  it('reconnects immediately after an image error without waiting for the renewal timer', async () => {
    vi.spyOn(api, 'get').mockImplementation(async url =>
      url.includes('/cameras')
        ? { enabled: true, cameras: [{ name: 'feeder', online: true, fps: 4 }] }
        : { data: [] }
    );
    renderTyped(CameraLivePage);
    const image = await screen.findByRole('img', { name: 'feeder' });
    const source = image.getAttribute('src');
    await fireEvent.error(image);
    await fireEvent.click(screen.getByRole('button', { name: 'common.retry' }));
    await waitFor(() => expect(image.getAttribute('src')).not.toBe(source));
    expect(screen.queryByRole('button', { name: 'common.retry' })).not.toBeInTheDocument();
  });

  it('releases the stream when hidden and reconnects when visible', async () => {
    let hidden = false;
    vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden);
    vi.spyOn(api, 'get').mockImplementation(async url =>
      url.includes('/cameras')
        ? { enabled: true, cameras: [{ name: 'feeder', online: true, fps: 4 }] }
        : { data: [] }
    );
    renderTyped(CameraLivePage);
    await screen.findByRole('img', { name: 'feeder' });
    hidden = true;
    await fireEvent(document, new Event('visibilitychange'));
    await waitFor(() => expect(screen.queryByRole('img')).not.toBeInTheDocument());
    hidden = false;
    await fireEvent(document, new Event('visibilitychange'));
    expect(await screen.findByRole('img', { name: 'feeder' })).toBeInTheDocument();
  });

  it('shows a disconnected state after status fails instead of claiming it is connecting', async () => {
    let offline = false;
    vi.spyOn(api, 'get').mockImplementation(async url => {
      if (!url.includes('/cameras')) return { data: [] };
      if (offline) throw new Error('Frigate unavailable');
      return { enabled: true, cameras: [{ name: 'feeder', online: true, fps: 4 }] };
    });
    renderTyped(CameraLivePage);
    await screen.findByRole('img', { name: 'feeder' });
    offline = true;
    await fireEvent.click(screen.getByRole('button', { name: 'common.refresh' }));
    expect(await screen.findByText('cameraLive.connectionError')).toBeInTheDocument();
    expect(screen.queryByText('cameraLive.connecting')).not.toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'common.retry' })).toBeInTheDocument();
  });

  it('keeps saved species observations visible when Frigate is unreachable', async () => {
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('Server error'))
      .mockResolvedValue({
        data: [
          {
            id: 8,
            camera: 'feeder',
            commonName: 'Blue Jay',
            scientificName: 'Cyanocitta cristata',
            startTime: '2026-11-13T22:45:30Z',
            detectionConfidence: 0.84375,
            classificationConfidence: null,
            classificationSource: 'frigate_sublabel',
            snapshotStatus: 'available',
          },
        ],
      });
    renderTyped(CameraLivePage);
    expect(await screen.findByText('Blue Jay')).toBeInTheDocument();
    expect(screen.getByText('Cyanocitta cristata')).toBeInTheDocument();
    expect(screen.getByText('84%')).toBeInTheDocument();
    expect(screen.getByText('common.unknown')).toBeInTheDocument();
    expect(screen.getByText('cameraLive.connectionError')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'cameraLive.snapshot' })).toHaveAttribute(
      'href',
      '/api/v2/visual-detections/8/snapshot'
    );
  });

  it('Accessibility: offers labelled source quality and explains the unsupported-browser fallback', async () => {
    vi.spyOn(api, 'get').mockImplementation(async url =>
      url.includes('/cameras')
        ? { enabled: true, cameras: [{ name: 'feeder', online: true, fps: 5 }] }
        : { data: [] }
    );
    renderTyped(CameraLivePage);
    await screen.findByRole('img', { name: 'feeder' });
    await fireEvent.click(screen.getByRole('combobox', { name: 'cameraLive.qualityLabel' }));
    await fireEvent.click(screen.getByRole('option', { name: 'cameraLive.sourceQuality' }));
    expect(await screen.findByText('cameraLive.sourceFallback')).toBeInTheDocument();
    expect(await screen.findByRole('img', { name: 'feeder' })).toBeInTheDocument();
    expect(screen.getByText('cameraLive.detectionFps: 5.0')).toBeInTheDocument();
  });
});
