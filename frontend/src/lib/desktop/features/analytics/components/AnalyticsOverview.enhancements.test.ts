import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import { api } from '$lib/utils/api';
import AnalyticsOverview from './AnalyticsOverview.svelte';
import NewSpeciesTimelineChart from './charts/d3/NewSpeciesTimelineChart.svelte';
import { makeAnalyticsParams } from '../registry/__tests__/registryFixtures';

vi.mock('$lib/utils/api', () => ({
  api: { get: vi.fn() },
  ApiError: class ApiError extends Error {},
}));

afterEach(() => {
  cleanup();
  vi.mocked(api.get).mockReset();
  localStorage.removeItem('analytics.summary.speciesLimit');
  localStorage.removeItem('analytics.summary.speciesChartStyle');
});

describe('Summary chart controls', () => {
  it('switches top 10 to a pie including other species, and keeps top 20 as bars', async () => {
    const rows = Array.from({ length: 30 }, (_, index) => ({
      scientific_name: `Species ${index}`,
      common_name: `Bird ${index}`,
      count: 30 - index,
      avg_confidence: 0.8,
    }));
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('/species/summary'))
        return url.includes('limit=20') ? rows.slice(0, 20) : rows;
      if (url.includes('/time/daily')) return { data: [] };
      return [];
    });
    const view = renderTyped(AnalyticsOverview, { props: { params: makeAnalyticsParams() } });
    const control = view.getByLabelText('analytics.summary.speciesLimit');
    const card = control.closest('.card');
    if (!card) throw new Error('Expected species chart');
    await waitFor(() => expect(card.querySelectorAll('rect.bar')).toHaveLength(10));
    await fireEvent.change(view.getByLabelText('analytics.summary.chartStyle'), {
      target: { value: 'pie' },
    });
    await waitFor(() => expect(card.querySelectorAll('.pie-slice')).toHaveLength(11));
    expect(card.textContent).toContain('analytics.patterns.otherSpecies');
    expect(JSON.parse(localStorage.getItem('analytics.summary.speciesChartStyle') ?? 'null')).toBe(
      'pie'
    );
    await fireEvent.change(control, { target: { value: '20' } });
    await waitFor(() => expect(card.querySelectorAll('rect.bar')).toHaveLength(20));
    expect(card.querySelectorAll('.pie-slice')).toHaveLength(0);
    expect(view.queryByLabelText('analytics.summary.chartStyle')).not.toBeInTheDocument();
    await fireEvent.change(control, { target: { value: '10' } });
    await waitFor(() => expect(card.querySelectorAll('.pie-slice')).toHaveLength(11));
  });

  it('migrates a saved top-25 preference to top 20', async () => {
    localStorage.setItem('analytics.summary.speciesLimit', JSON.stringify(25));
    vi.mocked(api.get).mockImplementation(async (url: string) =>
      url.includes('/time/daily') ? { data: [] } : []
    );
    const view = renderTyped(AnalyticsOverview, { props: { params: makeAnalyticsParams() } });
    expect(view.getByLabelText('analytics.summary.speciesLimit')).toHaveValue('20');
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem('analytics.summary.speciesLimit') ?? 'null')).toBe(20)
    );
    expect(view.container.querySelector('option[value="25"]')).not.toBeInTheDocument();
  });

  it.each([
    ['before noon', new Date(2026, 10, 10, 8)],
    ['at midnight', new Date(2026, 10, 10)],
  ])(
    'keeps the final daily trend point visible when the range ends %s',
    async (_label, endDate) => {
      vi.mocked(api.get).mockImplementation(async (url: string) =>
        url.includes('/time/daily')
          ? {
              data: [
                { date: '2026-11-01', count: 5 },
                { date: '2026-11-10', count: 8 },
              ],
            }
          : []
      );
      const { container } = renderTyped(AnalyticsOverview, {
        props: {
          params: { ...makeAnalyticsParams(), startDate: new Date(2026, 10, 1), endDate },
        },
      });
      await waitFor(() => expect(container.querySelectorAll('.line-point')).toHaveLength(2));
      const chart = container.querySelector('.line-chart');
      const points = chart?.querySelectorAll('.line-point');
      const finalPoint = points?.[1];
      if (!finalPoint) throw new Error('Expected final daily trend point');
      const clipWidth = Number(chart?.querySelector('clipPath rect')?.getAttribute('width'));
      const center = Number(finalPoint.getAttribute('cx'));
      const radius = Number(finalPoint.getAttribute('r'));
      expect(center + radius).toBeLessThanOrEqual(clipWidth);
      expect(center - radius).toBeGreaterThanOrEqual(0);
    }
  );

  it('weights average confidence by every detection, independently of the species chart limit', async () => {
    const rows = [
      { scientific_name: 'Bird A', common_name: 'A', count: 90, avg_confidence: 0.9 },
      { scientific_name: 'Bird B', common_name: 'B', count: 10, avg_confidence: 0.1 },
    ];
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('/species/summary'))
        return url.includes('limit=20') ? rows.slice(0, 1) : rows;
      if (url.includes('/time/daily')) return { data: [] };
      return [];
    });
    const { getByText } = renderTyped(AnalyticsOverview, {
      props: { params: makeAnalyticsParams() },
    });
    const card = getByText('analytics.stats.avgConfidence').closest('.card');
    await waitFor(() => expect(card?.textContent).toContain('82.0%'));
  });

  it('does not display zero confidence for an empty period', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) =>
      url.includes('/time/daily') ? { data: [] } : []
    );
    const { getByText } = renderTyped(AnalyticsOverview, {
      props: { params: makeAnalyticsParams() },
    });
    const card = getByText('analytics.stats.avgConfidence').closest('.card');
    await waitFor(() => expect(card?.textContent).toContain('—'));
    expect(card?.textContent).not.toContain('0.0%');
  });

  it('shows all 24 hours with aggregated totals and zero-filled missing hours', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('/time/distribution/hourly')) {
        return [
          { hour: 0, count: 3 },
          { hour: 0, count: 2 },
          { hour: 23, count: 7 },
        ];
      }
      if (url.includes('/time/daily')) return { data: [] };
      return [];
    });
    const { getByText } = renderTyped(AnalyticsOverview, {
      props: { params: makeAnalyticsParams() },
    });
    const card = getByText('analytics.summary.hourlyTitle').closest('.card');
    if (!card) throw new Error('Expected hourly chart card');
    await waitFor(() => expect(card.querySelectorAll('rect.bar')).toHaveLength(24));
    expect(card.textContent).toContain('00');
    expect(card.textContent).toContain('23');
    const bars = card.querySelectorAll('rect.bar');
    expect(Number(bars[0].getAttribute('height'))).toBeGreaterThan(0);
    expect(Number(bars[1].getAttribute('height'))).toBe(0);
    expect(Number(bars[23].getAttribute('height'))).toBeGreaterThan(
      Number(bars[0].getAttribute('height'))
    );
  });

  it('keeps first-heard markers inside a single selected calendar day', async () => {
    const day = new Date(2026, 10, 9);
    const { container } = renderTyped(NewSpeciesTimelineChart, {
      props: {
        dateRange: [day, day],
        data: [
          {
            commonName: 'New bird',
            scientificName: 'New species',
            firstHeard: new Date(2026, 10, 9, 12),
          },
        ],
      },
    });
    await waitFor(() => expect(container.querySelector('.timeline-marker')).not.toBeNull());
    const marker = container.querySelector('.timeline-marker');
    expect(Number(marker?.getAttribute('x'))).toBe(0);
    expect(Number(marker?.getAttribute('width'))).toBeGreaterThan(0);
  });

  it('loads all new-species pages and renders species beyond the former 20-row cap', async () => {
    const rows = Array.from({ length: 103 }, (_, index) => ({
      scientific_name: `New species ${index}`,
      common_name: `New bird ${index}`,
      first_heard_date: '2026-03-01',
    }));
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('/species/detections/new')) {
        const offset = Number(new URL(url, 'http://localhost').searchParams.get('offset'));
        return rows.slice(offset, offset + 100);
      }
      if (url.includes('/time/daily')) return { data: [] };
      return [];
    });
    const { container } = renderTyped(AnalyticsOverview, {
      props: { params: makeAnalyticsParams() },
    });
    await waitFor(() => expect(container.querySelectorAll('.timeline-marker')).toHaveLength(103));
    expect(container.textContent).toContain('New bird 102');
    expect(vi.mocked(api.get).mock.calls.some(([url]) => String(url).includes('offset=100'))).toBe(
      true
    );
  });

  it('requests up to 20 species once and switches the displayed ranking without refetching', async () => {
    const species = Array.from({ length: 30 }, (_, index) => ({
      scientific_name: `Species ${index}`,
      common_name: `Bird ${index}`,
      count: 30 - index,
      avg_confidence: 0.8,
    }));
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes('/species/summary'))
        return url.includes('limit=20') ? species.slice(0, 20) : species;
      if (url.includes('/time/daily')) return { data: [{ date: '2026-03-01', count: 20 }] };
      return [];
    });
    const { getByLabelText } = renderTyped(AnalyticsOverview, {
      props: { params: makeAnalyticsParams() },
    });
    const control = getByLabelText('analytics.summary.speciesLimit');
    const card = control.closest('.card');
    if (!card) throw new Error('Expected top-species chart card');
    await waitFor(() => expect(card.querySelectorAll('rect.bar')).toHaveLength(10));
    const calls = vi.mocked(api.get).mock.calls.length;
    await fireEvent.change(control, { target: { value: '20' } });
    await waitFor(() => expect(card.querySelectorAll('rect.bar')).toHaveLength(20));
    expect(vi.mocked(api.get).mock.calls.length).toBe(calls);
    expect(vi.mocked(api.get).mock.calls.some(([url]) => String(url).includes('limit=20'))).toBe(
      true
    );
    expect(JSON.parse(localStorage.getItem('analytics.summary.speciesLimit') ?? 'null')).toBe(20);
  });
});
