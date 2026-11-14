import { describe, it, expect, afterEach, vi } from 'vitest';
import { CHART_REGISTRY } from './charts';
import ConfidenceHistogram from '../components/ConfidenceHistogram.svelte';
import { makeAnalyticsParams } from './__tests__/registryFixtures';
import {
  confidenceSearch,
  histogramBars,
  percentChange,
  precedingPeriod,
} from '../utils/confidenceAnalytics';

import { confidenceFixture } from './__tests__/confidenceFixture';

afterEach(() => vi.unstubAllGlobals());

describe('confidence analytics', () => {
  it('enhances the existing registry entry and allows a single species', () => {
    const entry = CHART_REGISTRY.find(chart => chart.id === 'confidence-distribution');
    expect(entry?.component).toBe(ConfidenceHistogram);
    expect(entry?.supports.species).toBe(true);
    expect(entry?.minDataPoints).toBe(1);
    expect(CHART_REGISTRY.filter(chart => chart.id === 'confidence-distribution')).toHaveLength(1);
  });
  it('uses 5% bins and the complete count as percentage denominator', () => {
    const row = confidenceFixture();
    const counts = histogramBars(row, false);
    const percentages = histogramBars(row, true);
    expect(counts).toHaveLength(20);
    expect(counts[4]).toEqual({ label: '20\u201325%', value: 1 });
    expect(percentages[4].value).toBe(25);
    expect(percentages.reduce((sum, bar) => sum + bar.value, 0)).toBe(100);
    expect(counts[19].label).toBe('95\u2013100%');
    expect(
      histogramBars(confidenceFixture({ count: 0, bins: Array(20).fill(0) }), true).every(
        bar => bar.value === 0
      )
    ).toBe(true);
  });
  it('sends every selected species and the full date range', () => {
    const params = makeAnalyticsParams({ species: ['Parus major', 'Turdus merula'] });
    const search = confidenceSearch(params);
    expect(search.getAll('species')).toEqual(params.species);
    expect(search.get('format')).toBe('analysis');
    expect(search.has('limit')).toBe(false);
    expect(search.has('start_date')).toBe(true);
  });
  it('preserves cancellation and rejects malformed aggregates', async () => {
    const controller = new AbortController();
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => [{ count: 1 }] });
    vi.stubGlobal('fetch', fetch);
    const entry = CHART_REGISTRY.find(chart => chart.id === 'confidence-distribution');
    await expect(entry?.fetch(makeAnalyticsParams(), controller.signal)).rejects.toThrow(
      'Invalid confidence analytics'
    );
    expect(fetch.mock.calls[0][1].signal).toBe(controller.signal);
  });
  it('compares calendar periods across DST and avoids dividing by zero', () => {
    const previous = precedingPeriod(
      makeAnalyticsParams({ startDate: new Date(2026, 10, 1), endDate: new Date(2026, 10, 7) })
    );
    expect(previous.start).toBe('2026-10-25');
    expect(previous.end).toBe('2026-10-31');
    expect(percentChange(72, 100)).toBeCloseTo(-28);
    expect(percentChange(10, 0)).toBeNull();
  });
});
