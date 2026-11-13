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

  it('uses the same-origin proxy and releases it when paused', async () => {
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
});
