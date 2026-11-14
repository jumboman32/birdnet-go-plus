import { getLocalDateString } from '$lib/utils/date';

const PERCENT = 100;
const TOP_SPECIES_COUNT = 5;
const COVERAGE_TARGET = 0.8;
const HOURS_PER_DAY = 24;
const WINDOW_HOURS = 3;
const MILLISECONDS_PER_DAY = 86_400_000;

/** Species summary fields used for detection-share analysis, across all recorded models. */
export interface PatternSpecies {
  scientific_name?: string;
  common_name: string;
  count: number;
}

/** A species' share of recorded detections, not its population abundance. */
export interface SpeciesShare {
  key: string;
  scientific: string;
  name: string;
  count: number;
  share: number;
}

/** Detection concentration and Shannon effective diversity for a selected period. */
export interface SpeciesBalance {
  total: number;
  species: SpeciesShare[];
  effective: number;
  topFiveShare: number;
  speciesForCoverage: number;
}

/** Calendar periods of equal length, omitting today and future dates. */
export interface ComparisonWindow {
  currentStart: string;
  currentEnd: string;
  previousStart: string;
  previousEnd: string;
}

function midnight(date: Date): Date {
  const result = new Date(date);
  result.setHours(0, 0, 0, 0);
  return result;
}

function calendarStamp(date: Date): number {
  const result = new Date(0);
  result.setUTCFullYear(date.getFullYear(), date.getMonth(), date.getDate());
  result.setUTCHours(0, 0, 0, 0);
  return result.getTime();
}

/** Construct the adjacent comparison periods using calendar arithmetic, including across DST. */
export function comparisonWindow(
  start: Date,
  end: Date,
  now = new Date()
): ComparisonWindow | null {
  if (![start, end, now].every(date => Number.isFinite(date.getTime()))) return null;
  const first = midnight(start);
  const last = midnight(end);
  const today = midnight(now);
  if (last >= today) {
    last.setTime(today.getTime());
    last.setDate(last.getDate() - 1);
  }
  if (last < first) return null;
  const days = (calendarStamp(last) - calendarStamp(first)) / MILLISECONDS_PER_DAY + 1;
  const previousEnd = new Date(first);
  previousEnd.setDate(previousEnd.getDate() - 1);
  const previousStart = new Date(first);
  previousStart.setDate(previousStart.getDate() - days);
  return {
    currentStart: getLocalDateString(first),
    currentEnd: getLocalDateString(last),
    previousStart: getLocalDateString(previousStart),
    previousEnd: getLocalDateString(previousEnd),
  };
}

/** Aggregate repeated species identities and calculate detection-weighted diversity and shares. */
export function speciesBalance(rows: PatternSpecies[]): SpeciesBalance {
  const combined = new Map<string, SpeciesShare>();
  for (const row of rows) {
    if (!Number.isSafeInteger(row.count) || row.count <= 0) continue;
    const scientific = row.scientific_name?.trim() ?? '';
    const key = scientific || row.common_name.trim();
    if (!key) continue;
    const existing = combined.get(key);
    if (existing) existing.count += row.count;
    else
      combined.set(key, {
        key,
        scientific,
        name: row.common_name || scientific,
        count: row.count,
        share: 0,
      });
  }
  const species = [...combined.values()].sort(
    (a, b) => b.count - a.count || a.key.localeCompare(b.key)
  );
  const total = species.reduce((sum, row) => sum + row.count, 0);
  let entropy = 0;
  let cumulative = 0;
  let speciesForCoverage = 0;
  for (const row of species) {
    const probability = row.count / total;
    row.share = probability * PERCENT;
    entropy -= probability * Math.log(probability);
    if (cumulative < total * COVERAGE_TARGET) speciesForCoverage++;
    cumulative += row.count;
  }
  return {
    total,
    species,
    effective: total > 0 ? Math.exp(entropy) : 0,
    topFiveShare: species.slice(0, TOP_SPECIES_COUNT).reduce((sum, row) => sum + row.share, 0),
    speciesForCoverage,
  };
}

/** Relative change is undefined for an empty baseline, including when both periods are empty. */
export function relativeChange(current: number, previous: number): number | null {
  return previous > 0 ? ((current - previous) / previous) * PERCENT : null;
}

/** A change in a species' share of detections, measured in percentage points. */
export interface SpeciesMovement {
  species: SpeciesShare;
  previousShare: number;
  currentShare: number;
  change: number;
}

/** Rank species by share change; suppress ranking when either period lacks recorded detections. */
export function speciesMovements(
  current: SpeciesBalance,
  previous: SpeciesBalance
): SpeciesMovement[] {
  if (current.total === 0 || previous.total === 0) return [];
  const before = new Map(previous.species.map(row => [row.key, row]));
  const after = new Map(current.species.map(row => [row.key, row]));
  const keys = new Set([...before.keys(), ...after.keys()]);
  const rows: SpeciesMovement[] = [];
  for (const key of keys) {
    const old = before.get(key);
    const fresh = after.get(key);
    const species = fresh ?? old;
    if (!species) continue;
    const previousShare = old?.share ?? 0;
    const currentShare = fresh?.share ?? 0;
    rows.push({ species, previousShare, currentShare, change: currentShare - previousShare });
  }
  return rows.sort(
    (a, b) => Math.abs(b.change) - Math.abs(a.change) || a.species.key.localeCompare(b.species.key)
  );
}

/** Find the most active three-hour window on the cyclic 24-hour clock, with earliest-hour ties. */
export function peakActivityWindow(
  rows: { hour: number; count: number }[]
): { start: number; end: number; share: number } | null {
  const hours = new Array<number>(HOURS_PER_DAY).fill(0);
  for (const row of rows) {
    if (
      !Number.isInteger(row.hour) ||
      row.hour < 0 ||
      row.hour >= HOURS_PER_DAY ||
      !Number.isSafeInteger(row.count) ||
      row.count < 0
    )
      continue;
    const count = hours.at(row.hour) ?? 0;
    hours.splice(row.hour, 1, count + row.count);
  }
  const total = hours.reduce((sum, count) => sum + count, 0);
  if (!total) return null;
  let peakStart = 0;
  let peakCount = -1;
  for (let start = 0; start < HOURS_PER_DAY; start++) {
    let count = 0;
    for (let offset = 0; offset < WINDOW_HOURS; offset++)
      count += hours.at((start + offset) % HOURS_PER_DAY) ?? 0;
    if (count > peakCount) {
      peakStart = start;
      peakCount = count;
    }
  }
  return {
    start: peakStart,
    end: (peakStart + WINDOW_HOURS) % HOURS_PER_DAY,
    share: (peakCount / total) * PERCENT,
  };
}
