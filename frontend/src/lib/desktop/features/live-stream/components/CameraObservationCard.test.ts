import { describe, it, expect } from 'vitest';
import { fireEvent, screen } from '@testing-library/svelte';
import { createComponentTestFactory, renderTyped } from '../../../../../test/render-helpers';
import CameraObservationCard from './CameraObservationCard.svelte';
import type { VisualObservation } from './CameraObservationResults.svelte';

describe('Camera observation card', () => {
  it('retries a failed snapshot only after its event state changes', async () => {
    const observation: VisualObservation = {
      id: 8,
      camera: 'feeder',
      commonName: 'Blue Jay',
      startTime: '2026-11-13T22:45:30Z',
      snapshotStatus: 'available',
    };
    const { rerender } = createComponentTestFactory(CameraObservationCard).render({ observation });
    await fireEvent.error(screen.getByRole('img', { name: 'Blue Jay' }));
    await rerender({ observation: { ...observation } });
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    await rerender({ observation: { ...observation, endTime: '2026-11-13T22:46:00Z' } });
    expect(screen.getByRole('img', { name: 'Blue Jay' })).toBeInTheDocument();
  });
  it('uses the saved snapshot and exposes its source without audio playback controls', () => {
    renderTyped(CameraObservationCard, {
      props: {
        observation: {
          id: 8,
          camera: 'feeder',
          commonName: 'Blue Jay',
          scientificName: 'Cyanocitta cristata',
          startTime: '2026-11-13T22:45:30Z',
          snapshotStatus: 'available',
          detectionConfidence: 0.84375,
        },
      },
    });
    expect(screen.getByRole('img', { name: 'Blue Jay' })).toHaveAttribute(
      'src',
      '/api/v2/visual-detections/8/snapshot'
    );
    expect(screen.getByText('Frigate · feeder')).toBeInTheDocument();
    expect(screen.getByText('84%')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
