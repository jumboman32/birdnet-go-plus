import { parseLocalDateString } from '$lib/utils/date';
import type { TrendResponse } from './analyticsTransforms';

export const DEFAULT_TOP_SPECIES_LIMIT = 10;
export const MAX_TOP_SPECIES_LIMIT = 20;
const DAYS_PER_WEEK = 7;
const MONDAY_OFFSET = 6;
const MILLISECONDS_PER_CALENDAR_DAY = 86400000;

/** Confirmed server availability; incomplete history yields a lower bound, not downtime. */
export interface PeriodUptime {
  percentage: number;
  complete: boolean;
}

/** Intersect the current process session with the elapsed portion of an inclusive date range. */
export function summarizeUptime(
  sessionStart: string | Date,
  sessionSeconds: number,
  start: Date,
  end: Date
): PeriodUptime | null {
  const millisecondsPerSecond = 1000;
  const startedAt = new Date(sessionStart).getTime();
  const observedAt = startedAt + sessionSeconds * millisecondsPerSecond;
  const periodStart = new Date(start);
  periodStart.setHours(0, 0, 0, 0);
  const periodEnd = new Date(end);
  periodEnd.setHours(0, 0, 0, 0);
  periodEnd.setDate(periodEnd.getDate() + 1);
  if (
    !Number.isFinite(startedAt) ||
    !Number.isFinite(sessionSeconds) ||
    sessionSeconds < 0 ||
    !Number.isFinite(periodStart.getTime()) ||
    !Number.isFinite(periodEnd.getTime()) ||
    periodEnd <= periodStart ||
    Math.min(periodEnd.getTime(), observedAt) <= periodStart.getTime()
  )
    return null;
  const elapsedEnd = Math.min(periodEnd.getTime(), observedAt);
  const knownStart = Math.max(periodStart.getTime(), startedAt);
  if (knownStart >= elapsedEnd) return null;
  const percentScale = 100;
  return {
    percentage: ((elapsedEnd - knownStart) / (elapsedEnd - periodStart.getTime())) * percentScale,
    complete: startedAt <= periodStart.getTime(),
  };
}

/** Derive host availability using the server clock, so an application restart does not reset it. */
export function summarizeSystemUptime(
  systemSeconds: number,
  appStart: string,
  appSeconds: number,
  start: Date,
  end: Date
): PeriodUptime | null {
  const millisecondsPerSecond = 1000;
  const appStartedAt = new Date(appStart).getTime();
  if (
    !Number.isFinite(systemSeconds) ||
    systemSeconds < 0 ||
    !Number.isFinite(appSeconds) ||
    appSeconds < 0 ||
    !Number.isFinite(appStartedAt)
  )
    return null;
  const observedAt = appStartedAt + appSeconds * millisecondsPerSecond;
  const systemStartedAt = new Date(observedAt - systemSeconds * millisecondsPerSecond);
  return summarizeUptime(systemStartedAt, systemSeconds, start, end);
}

/** Activity over calendar dates, independent of recording uptime. Weekdays start on Monday. */
export interface SummaryActivity {
  calendarDays: number;
  activeDays: number;
  detections: number;
  dailyAverage: number;
  busiest: { date: Date; count: number } | null;
  weekdays: { day: number; count: number; calendarDays: number; average: number }[];
}

function calendarStamp(date: Date): number {
  const stamp = new Date(0);
  stamp.setUTCFullYear(date.getFullYear(), date.getMonth(), date.getDate());
  stamp.setUTCHours(0, 0, 0, 0);
  return stamp.getTime();
}

/** Aggregate all daily API rows, including duplicate dates; omit invalid or unavailable statistics. */
export function summarizeActivity(
  response: TrendResponse | null,
  start: Date,
  end: Date
): SummaryActivity | null {
  if (
    !response ||
    !Array.isArray(response.data) ||
    !Number.isFinite(start.getTime()) ||
    !Number.isFinite(end.getTime())
  )
    return null;
  const first = calendarStamp(start);
  const last = calendarStamp(end);
  if (last < first) return null;
  const calendarDays = Math.round((last - first) / MILLISECONDS_PER_CALENDAR_DAY) + 1;
  const startWeekday = (start.getDay() + MONDAY_OFFSET) % DAYS_PER_WEEK;
  const weekdays = Array.from({ length: DAYS_PER_WEEK }, (_, day) => ({
    day,
    count: 0,
    calendarDays: Math.floor(calendarDays / DAYS_PER_WEEK),
    average: 0,
  }));
  for (let extra = 0; extra < calendarDays % DAYS_PER_WEEK; extra++) {
    const day = weekdays.at((startWeekday + extra) % DAYS_PER_WEEK);
    if (day) day.calendarDays++;
  }
  const dates = new Map<number, { date: Date; count: number }>();
  for (const row of response.data) {
    if (typeof row.date !== 'string' || !Number.isSafeInteger(row.count) || row.count < 0)
      return null;
    const date = parseLocalDateString(row.date);
    if (!date || !Number.isFinite(date.getTime())) return null;
    const stamp = calendarStamp(date);
    if (stamp < first || stamp > last) continue;
    dates.set(stamp, { date, count: (dates.get(stamp)?.count ?? 0) + row.count });
  }
  let detections = 0;
  let activeDays = 0;
  let busiest: SummaryActivity['busiest'] = null;
  for (const entry of dates.values()) {
    detections += entry.count;
    if (entry.count > 0) {
      activeDays++;
      if (
        !busiest ||
        entry.count > busiest.count ||
        (entry.count === busiest.count && entry.date < busiest.date)
      )
        busiest = entry;
    }
    const weekday = weekdays.at((entry.date.getDay() + MONDAY_OFFSET) % DAYS_PER_WEEK);
    if (weekday) weekday.count += entry.count;
  }
  for (const weekday of weekdays)
    weekday.average = weekday.calendarDays > 0 ? weekday.count / weekday.calendarDays : 0;
  return {
    calendarDays,
    activeDays,
    detections,
    dailyAverage: detections / calendarDays,
    busiest,
    weekdays,
  };
}
