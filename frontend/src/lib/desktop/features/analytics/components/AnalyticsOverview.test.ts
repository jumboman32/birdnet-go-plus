import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { waitFor, cleanup } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../../../test/render-helpers';
import { api } from '$lib/utils/api';
import AnalyticsOverview from './AnalyticsOverview.svelte';
import type { AnalyticsParams } from '../registry/types';

// Drive successful and failed analytics requests through the API mock.
vi.mock('$lib/utils/api', () => ({
  api: { get: vi.fn() },
  ApiError: class ApiError extends Error {},
}));

// D3 charts are irrelevant to error handling and
// pull in heavy/contextual dependencies, so stub them out.
vi.mock('./charts/d3/BarChart.svelte');
vi.mock('./charts/d3/LineChart.svelte');
vi.mock('./charts/d3/NewSpeciesTimelineChart.svelte');

const overviewTest = createComponentTestFactory(AnalyticsOverview);

// Two distinct date ranges. Switching between them changes the hub's resolved
// range, which is what drives a refetch (the old standalone page used a form).
function makeParams(start: string, end: string): AnalyticsParams {
  return {
    range: 'custom',
    start,
    end,
    species: [],
    source: '',
    startDate: new Date(`${start}T00:00:00`),
    endDate: new Date(`${end}T00:00:00`),
  };
}
const RANGE_A = makeParams('2024-01-01', '2024-01-07');

describe('AnalyticsOverview total-failure error banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.get).mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  // When every request fails the user must see a global error, not just
  // empty charts. Each fetcher rethrows after logging so fetchData can tell a
  // total failure apart from partial/empty results.
  it('shows the error banner when every request fails', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('network down'));

    const { container } = overviewTest.render({ params: RANGE_A });

    await waitFor(() => {
      expect(container.textContent).toContain('analytics.loadingError');
    });
  });

  // A partial failure is communicated by the affected chart's own empty state,
  // so the global banner must stay hidden when at least one request succeeds.
  it('does not show the error banner on a partial failure', async () => {
    vi.mocked(api.get).mockImplementation((url: string): Promise<unknown> => {
      // Only the new-species endpoint fails; everything else resolves.
      if (url.includes('/api/v2/analytics/species/detections/new')) {
        return Promise.reject(new Error('boom'));
      }
      if (url.includes('/api/v2/analytics/time/daily')) {
        return Promise.resolve({ data: [] });
      }
      return Promise.resolve([]);
    });

    const { container } = overviewTest.render({ params: RANGE_A });

    // Let all five fetchers settle, then a macrotask so the error-state commit
    // (if any) and the Svelte DOM flush complete before asserting absence.
    await waitFor(() => {
      expect(vi.mocked(api.get).mock.calls.length).toBeGreaterThanOrEqual(5);
    });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(container.textContent).not.toContain('analytics.loadingError');
  });
});
