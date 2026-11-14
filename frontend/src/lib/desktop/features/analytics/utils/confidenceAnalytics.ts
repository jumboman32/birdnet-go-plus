import { buildAppUrl } from '$lib/utils/urlHelpers';
import { formatDateForAPI } from '../registry/analyticsParams';
import type { AnalyticsParams } from '../registry/types';

export const CONFIDENCE_BINS = 20;
export const REVIEW_PAGE_SIZE = 25;

/** This feature is unavailable until the enhanced database is active. */
export class ConfidenceAnalyticsUnavailableError extends Error {}

/** Exact server aggregates; one primary identification per saved detection, separated by model. */
export interface ConfidenceSeries {
  modelId: number;
  model: string;
  species: string;
  commonName?: string;
  count: number;
  bins: number[];
  average: number;
  median: number;
  low: number;
  middle: number;
  high: number;
  uniqueSpecies: number;
  topSpecies: string;
  topCommonName?: string;
  topCount: number;
}

export interface ConfidenceReview {
  id: number;
  species: string;
  commonName?: string;
  confidence: number;
  detectedAt: number;
}

/** Validate a review row before rendering the keyed list. */
export function isConfidenceReview(value: unknown): value is ConfidenceReview {
  if (!value || typeof value !== 'object') return false;
  return (
    numericField(value, 'id', 1, Number.MAX_SAFE_INTEGER, true) &&
    numericField(value, 'detectedAt', 0, Number.MAX_SAFE_INTEGER, true) &&
    numericField(value, 'confidence', 0, 1) &&
    'species' in value &&
    typeof value.species === 'string' &&
    (!('commonName' in value) || typeof value.commonName === 'string')
  );
}

/** Shared endpoint builder preserves station-local date strings and repeatable species filters. */
export function confidenceSearch(params: AnalyticsParams): URLSearchParams {
  const search = new URLSearchParams({
    format: 'analysis',
    start_date: formatDateForAPI(params.startDate),
    end_date: formatDateForAPI(params.endDate),
  });
  params.species.forEach(name => search.append('species', name));
  return search;
}

function numericField(
  value: object,
  key: string,
  min: number,
  max: number,
  integer = false
): boolean {
  const number: unknown = Reflect.get(value, key);
  return (
    typeof number === 'number' &&
    Number.isFinite(number) &&
    number >= min &&
    number <= max &&
    (!integer || Number.isSafeInteger(number))
  );
}

function isSeries(value: unknown): value is ConfidenceSeries {
  if (!value || typeof value !== 'object') return false;
  if (!('bins' in value) || !Array.isArray(value.bins) || value.bins.length !== CONFIDENCE_BINS)
    return false;
  const validBins = value.bins.every(
    n => typeof n === 'number' && Number.isSafeInteger(n) && n >= 0
  );
  if (!validBins || !numericField(value, 'count', 1, Number.MAX_SAFE_INTEGER, true)) return false;
  const count: number = Reflect.get(value, 'count');
  return (
    numericField(value, 'modelId', 0, Number.MAX_SAFE_INTEGER, true) &&
    numericField(value, 'average', 0, 1) &&
    numericField(value, 'median', 0, 1) &&
    ['low', 'middle', 'high', 'uniqueSpecies', 'topCount'].every(key =>
      numericField(value, key, 0, count, true)
    ) &&
    value.bins.reduce((sum, number) => sum + number, 0) === count &&
    ['model', 'species', 'topSpecies'].every(key => typeof Reflect.get(value, key) === 'string')
  );
}

/** Fetch aggregate data only; cancellation is owned by the chart/page lifecycle. */
export async function fetchConfidenceAnalysis(
  params: AnalyticsParams,
  signal?: AbortSignal
): Promise<ConfidenceSeries[]> {
  const response = await fetch(
    buildAppUrl(`/api/v2/analytics/confidence/distribution?${confidenceSearch(params)}`),
    { signal }
  );
  if (response.status === 501)
    throw new ConfidenceAnalyticsUnavailableError('Enhanced database required');
  if (!response.ok) throw new Error(`Confidence analytics: HTTP ${response.status}`);
  const data: unknown = await response.json();
  if (!Array.isArray(data) || !data.every(isSeries))
    throw new Error('Invalid confidence analytics response');
  return data;
}

/** Counts and normalized percentages use the same complete, server-aggregated denominator. */
export function histogramBars(series: ConfidenceSeries, normalized: boolean) {
  const width = 100 / CONFIDENCE_BINS;
  return series.bins.map((count, index) => ({
    label: `${index * width}–${(index + 1) * width}%`,
    value: normalized ? (series.count > 0 ? (count / series.count) * 100 : 0) : count,
  }));
}

/** Equivalent preceding calendar period, safe across daylight-saving transitions. */
export function precedingPeriod(params: AnalyticsParams): AnalyticsParams {
  const start = new Date(params.startDate);
  const end = new Date(params.endDate);
  const MILLISECONDS_PER_CALENDAR_DAY = 86400000;
  const days =
    Math.round(
      (Date.UTC(end.getFullYear(), end.getMonth(), end.getDate()) -
        Date.UTC(start.getFullYear(), start.getMonth(), start.getDate())) /
        MILLISECONDS_PER_CALENDAR_DAY
    ) + 1;
  const previousEnd = new Date(start);
  previousEnd.setDate(previousEnd.getDate() - 1);
  start.setDate(start.getDate() - days);
  return {
    ...params,
    startDate: start,
    endDate: previousEnd,
    start: formatDateForAPI(start),
    end: formatDateForAPI(previousEnd),
  };
}

/** Omit percent change for an empty baseline instead of showing infinity. */
export function percentChange(current: number, previous: number): number | null {
  return previous > 0 ? ((current - previous) / previous) * 100 : null;
}
