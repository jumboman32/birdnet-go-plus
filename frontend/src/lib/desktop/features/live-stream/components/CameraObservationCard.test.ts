import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import CameraObservationCard from './CameraObservationCard.svelte';

describe('Camera observation card', () => {
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
