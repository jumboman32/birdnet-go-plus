import { describe, it, expect } from 'vitest';
import { fireEvent, screen } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import CameraObservationResults from './CameraObservationResults.svelte';

describe('Camera observation snapshots', () => {
  it('shows the event snapshot and falls back to its link when the image expires', async () => {
    renderTyped(CameraObservationResults, {
      props: {
        data: [
          {
            id: 8,
            camera: 'feeder',
            commonName: 'Blue Jay',
            startTime: '2026-11-13T22:45:30Z',
            snapshotStatus: 'available',
          },
        ],
      },
    });
    const image = screen.getByRole('img', { name: 'Blue Jay' });
    expect(image).toHaveAttribute('src', '/api/v2/visual-detections/8/snapshot');
    await fireEvent.error(image);
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'cameraLive.snapshot' })).toHaveAttribute(
      'href',
      '/api/v2/visual-detections/8/snapshot'
    );
  });
});
