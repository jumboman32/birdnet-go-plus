import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../../../test/render-helpers';
import { api } from '$lib/utils/api';
import { makeAnalyticsParams } from '../registry/__tests__/registryFixtures';
import SummaryPatterns from './SummaryPatterns.svelte';
const factory = createComponentTestFactory(SummaryPatterns);

vi.mock('$lib/utils/api', () => ({
  api: { get: vi.fn() },
  ApiError: class ApiError extends Error {},
}));

afterEach(() => {
  cleanup();
  vi.mocked(api.get).mockReset();
});

const species = [
  { scientific_name: 'Bird A', common_name: 'A', count: 20 },
  { scientific_name: 'Bird B', common_name: 'B', count: 80 },
];
const params = {
  ...makeAnalyticsParams(),
  startDate: new Date(2026, 2, 1),
  endDate: new Date(2026, 2, 2),
};

describe('Summary patterns', () => {
  it('renders previous-period changes and species share gains with links to the compared dates', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('start_date=2026-03-01'))
        return [
          { scientific_name: 'Bird A', common_name: 'A', count: 100 },
          { scientific_name: 'Bird B', common_name: 'B', count: 100 },
        ];
      return species;
    });
    const view = factory.render({
      props: { params, species, hourly: [], isLoading: false },
    });
    await waitFor(() => expect(view.getByText('+100.0%')).toBeInTheDocument());
    const link = view.getByRole('link', { name: 'A' });
    expect(link.getAttribute('href')).toContain('start_date=2026-03-01');
    expect(link.getAttribute('href')).toContain('end_date=2026-03-02');
    expect(link.closest('li')?.textContent).toContain('+30.0 pp');
  });

  it('does not invent percentage growth when the previous period has no detections', async () => {
    vi.mocked(api.get).mockResolvedValue([]);
    const view = factory.render({
      props: { params, species, hourly: [], isLoading: false },
    });
    await waitFor(() =>
      expect(view.getByText('analytics.patterns.noBaseline')).toBeInTheDocument()
    );
    expect(view.container.textContent).not.toContain('Infinity');
  });

  it('offers retry for failed comparisons without suppressing selected-period patterns', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('offline'));
    const view = factory.render({
      props: { params, species, hourly: [], isLoading: false },
    });
    await waitFor(() => expect(view.getByRole('alert')).toBeInTheDocument());
    expect(view.getByText('analytics.patterns.effective')).toBeInTheDocument();
    vi.mocked(api.get).mockResolvedValue([]);
    await fireEvent.click(view.getByRole('button', { name: 'common.retry' }));
    await waitFor(() =>
      expect(view.getByText('analytics.patterns.noBaseline')).toBeInTheDocument()
    );
  });

  it('discards late comparison responses after the date range changes', async () => {
    let resolveEarlier: ((value: typeof species) => void) | undefined;
    const earlier = new Promise<typeof species>(resolve => {
      resolveEarlier = resolve;
    });
    vi.mocked(api.get).mockImplementation(async (url: string) =>
      url.includes('start_date=2026-03-01') ? earlier : []
    );
    const view = factory.render({
      props: { params, species, hourly: [], isLoading: false },
    });
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2));
    await view.rerender({
      params: { ...params, startDate: new Date(2026, 3, 1), endDate: new Date(2026, 3, 2) },
      species,
      hourly: [],
      isLoading: false,
    });
    await waitFor(() =>
      expect(view.getByText('analytics.patterns.noBaseline')).toBeInTheDocument()
    );
    resolveEarlier?.(species);
    await earlier;
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(view.getByText('analytics.patterns.noBaseline')).toBeInTheDocument();
    expect(view.queryByText('+100.0%')).not.toBeInTheDocument();
  });
});
