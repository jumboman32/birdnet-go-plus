import { isPlainObject } from '$lib/utils/security';

/** Number of Jan-1-anchored seasonal periods in a complete calendar. */
export const CALENDAR_WEEK_COUNT = 52;
/** Minimum reporting frequency included by the expected-this-week filter. */
export const CALENDAR_EXPECTED_RATE = 0.02;
const CALENDAR_VERSION = 1;
const REFERENCE_YEAR = 2001;
const DAYS_PER_WEEK = 7;

/** One taxon's historical frequencies, seasonal events and station observations. */
export interface CalendarSpecies {
  scientific: string;
  name: string;
  rates: (number | null)[];
  heard: number[];
  counts: number[];
  arrivals: number[];
  departures: number[];
  peaks: number[];
  resident: boolean;
}

/** A private calendar snapshot produced by the local eBird exporter. */
export interface BirdCalendarReport {
  version: number;
  year: number;
  currentWeek: number;
  generated: string;
  timezone: string;
  region: string;
  radius: number;
  citation: string;
  firstHistoricalYear: number;
  lastHistoricalYear: number;
  species: CalendarSpecies[];
  samples: number[];
  years: number[];
  stationDays: number[];
  firstDetectionDate: string | null;
  lastDetectionDate: string | null;
  totalDetections: number;
}

function isCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

function isWeeklyCounts(value: unknown): value is number[] {
  return Array.isArray(value) && value.length === CALENDAR_WEEK_COUNT && value.every(isCount);
}

function isWeekList(value: unknown): value is number[] {
  return Array.isArray(value) && value.every(week => isCount(week) && week < CALENDAR_WEEK_COUNT);
}

function isSpecies(value: unknown): value is CalendarSpecies {
  return (
    isPlainObject(value) &&
    typeof value.scientific === 'string' &&
    typeof value.name === 'string' &&
    typeof value.resident === 'boolean' &&
    isWeeklyCounts(value.heard) &&
    isWeeklyCounts(value.counts) &&
    isWeekList(value.arrivals) &&
    isWeekList(value.departures) &&
    isWeekList(value.peaks) &&
    Array.isArray(value.rates) &&
    value.rates.length === CALENDAR_WEEK_COUNT &&
    value.rates.every(
      rate =>
        rate === null ||
        (typeof rate === 'number' && Number.isFinite(rate) && rate >= 0 && rate <= 1)
    )
  );
}

/** Validate imported data before it reaches reactive rendering or date formatting. */
export function isBirdCalendarReport(value: unknown): value is BirdCalendarReport {
  if (
    !isPlainObject(value) ||
    value.version !== CALENDAR_VERSION ||
    !isCount(value.year) ||
    value.year === 0 ||
    !isCount(value.currentWeek) ||
    value.currentWeek >= CALENDAR_WEEK_COUNT ||
    typeof value.generated !== 'string' ||
    Number.isNaN(Date.parse(value.generated)) ||
    typeof value.timezone !== 'string' ||
    typeof value.region !== 'string' ||
    typeof value.citation !== 'string' ||
    typeof value.radius !== 'number' ||
    !Number.isFinite(value.radius) ||
    value.radius <= 0 ||
    !isCount(value.firstHistoricalYear) ||
    !isCount(value.lastHistoricalYear) ||
    !isCount(value.totalDetections) ||
    !isWeeklyCounts(value.samples) ||
    !isWeeklyCounts(value.years) ||
    !isWeeklyCounts(value.stationDays) ||
    !Array.isArray(value.species) ||
    !value.species.every(isSpecies) ||
    new Set(value.species.map(bird => bird.scientific)).size !== value.species.length ||
    !(value.firstDetectionDate === null || typeof value.firstDetectionDate === 'string') ||
    !(value.lastDetectionDate === null || typeof value.lastDetectionDate === 'string')
  )
    return false;
  try {
    new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(new Date(value.generated));
    return true;
  } catch {
    return false;
  }
}

/** Format Jan-1 anchored seasonal periods, with December 31 in the final period. */
export function calendarWeekPeriod(week: number, locale: string): string {
  const start = new Date(REFERENCE_YEAR, 0, 1 + week * DAYS_PER_WEEK);
  const end =
    week === CALENDAR_WEEK_COUNT - 1
      ? new Date(REFERENCE_YEAR, 11, 31)
      : new Date(REFERENCE_YEAR, 0, DAYS_PER_WEEK + week * DAYS_PER_WEEK);
  const formatter = new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' });
  return `${formatter.format(start)} – ${formatter.format(end)}`;
}
