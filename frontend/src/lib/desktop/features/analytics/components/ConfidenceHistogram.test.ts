import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import ConfidenceHistogram from './ConfidenceHistogram.svelte';
import { confidenceFixture } from '../registry/__tests__/confidenceFixture';
import { makeAnalyticsParams } from '../registry/__tests__/registryFixtures';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('ConfidenceHistogram', () => {
  it('filters recorded models without pooling scores and toggles normalized values', async () => {
    const { container, getByLabelText } = renderTyped(ConfidenceHistogram, {
      props: {
        series: [confidenceFixture(), confidenceFixture({ modelId: 2, model: 'Perch' })],
        params: makeAnalyticsParams(),
      },
    });
    await waitFor(() => expect(container.querySelectorAll('rect.bar')).toHaveLength(40));
    await fireEvent.click(getByLabelText(/BirdNET/));
    await waitFor(() => expect(container.querySelectorAll('rect.bar')).toHaveLength(20));
    await fireEvent.click(getByLabelText('analytics.confidenceAnalysis.normalized'));
    await waitFor(() =>
      expect(container.querySelector('rect.bar[aria-label="20–25%: 25.0%"]')).toBeTruthy()
    );
  });

  it('Accessibility: keyboard bin selection requests only one bounded review page', async () => {
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        {
          id: 7,
          species: 'Parus major',
          commonName: 'Great Tit',
          confidence: 0.2,
          detectedAt: 1000,
        },
      ],
    });
    vi.stubGlobal('fetch', fetch);
    const { container, getByRole } = renderTyped(ConfidenceHistogram, {
      props: {
        series: [confidenceFixture()],
        params: makeAnalyticsParams(),
      },
    });
    await waitFor(() => expect(container.querySelectorAll('rect.bar')).toHaveLength(20));
    expect(container.querySelector('[role="group"][aria-label]')).toBeTruthy();
    await fireEvent.keyDown(getByRole('button', { name: '20–25%: 1' }), { key: 'Enter' });
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    const url = new URL(String(fetch.mock.calls[0][0]), 'http://localhost');
    expect(url.searchParams.get('format')).toBe('review');
    expect(url.searchParams.get('bin')).toBe('4');
    expect(url.searchParams.get('model')).toBe('1');
    expect(url.searchParams.get('start_date')).toBe('2026-03-01');
    await waitFor(() =>
      expect(getByRole('link', { name: 'Great Tit' })).toHaveAttribute('href', '/ui/detections/7')
    );
  });
});
