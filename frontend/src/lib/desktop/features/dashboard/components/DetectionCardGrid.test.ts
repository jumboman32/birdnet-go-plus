import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import DetectionCardGrid from './DetectionCardGrid.svelte';

describe('Recent audio and camera detections', () => {
  it('keeps overlapping IDs separate and orders cards by observation time', () => {
    const { container } = renderTyped(DetectionCardGrid, {
      props: {
        data: [
          {
            id: 8,
            date: '2026-11-13',
            time: '17:30:00',
            timestamp: '2026-11-13T22:30:00Z',
            beginTime: '',
            endTime: '',
            speciesCode: 'norcar',
            commonName: 'Northern Cardinal',
            scientificName: 'Cardinalis cardinalis',
            confidence: 0.9,
            verified: 'unverified',
            locked: false,
          },
        ],
        cameraData: [
          {
            id: 8,
            camera: 'feeder',
            commonName: 'Blue Jay',
            startTime: '2026-11-13T22:45:30Z',
            snapshotStatus: 'available',
          },
        ],
        onRefresh: vi.fn(),
      },
    });
    const cards = container.querySelectorAll('article');
    expect(cards).toHaveLength(2);
    expect(cards[0]).toHaveTextContent('Blue Jay');
    expect(screen.getByText('Northern Cardinal')).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Blue Jay' })).toHaveAttribute(
      'src',
      '/api/v2/visual-detections/8/snapshot'
    );
  });
});
