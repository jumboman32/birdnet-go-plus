import { describe, expect, it } from 'vitest';
import { summarizeActivity, summarizeSystemUptime, summarizeUptime } from './summaryMeasures';

describe('Period server uptime', () => {
  const start = new Date(2026, 10, 9);
  const session = new Date(2026, 10, 8).toString();
  it('reports availability for an elapsed period covered by the current session', () => {
    expect(summarizeUptime(session, 36 * 60 * 60, start, start)).toEqual({
      percentage: 100,
      complete: true,
    });
  });
  it('reports a lower bound when the session starts shortly after midnight', () => {
    const started = new Date(2026, 10, 9, 0, 0, 16);
    const result = summarizeUptime(started.toString(), 8 * 60 * 60, start, start);
    expect(result?.complete).toBe(false);
    expect(result?.percentage).toBeCloseTo((28800 / 28816) * 100, 8);
  });
  it('uses the selected calendar interval and excludes future time', () => {
    const started = new Date(2026, 10, 9, 12);
    expect(summarizeUptime(started.toString(), 36 * 60 * 60, start, start)).toEqual({
      percentage: 50,
      complete: false,
    });
    expect(summarizeUptime(session, 36 * 60 * 60, start, new Date(2026, 10, 10))).toEqual({
      percentage: 100,
      complete: true,
    });
  });
  it('does not invent uptime for unobserved dates or invalid data', () => {
    expect(summarizeUptime(session, 60, start, start)).toBeNull();
    expect(summarizeUptime('invalid', 60, start, start)).toBeNull();
    expect(summarizeUptime(session, -1, start, start)).toBeNull();
    expect(summarizeUptime(session, 3600, start, new Date(2026, 10, 8))).toBeNull();
  });

  it('keeps host uptime across an application restart', () => {
    const restartedAt = new Date(2026, 10, 9, 13);
    const result = summarizeSystemUptime(
      13 * 60 * 60 + 60,
      restartedAt.toString(),
      60,
      start,
      start
    );
    expect(result).toEqual({ percentage: 100, complete: true });
  });

  it('rejects invalid host or application uptime data', () => {
    expect(summarizeSystemUptime(-1, session, 60, start, start)).toBeNull();
    expect(summarizeSystemUptime(60, session, -1, start, start)).toBeNull();
    expect(summarizeSystemUptime(60, 'invalid', 60, start, start)).toBeNull();
  });
});

describe('Summary activity calculations', () => {
  it('uses inclusive calendar dates across DST and includes zero-detection dates in averages', () => {
    const result = summarizeActivity(
      {
        data: [
          { date: '2026-03-01', count: 14 },
          { date: '2026-03-02', count: 6 },
          { date: '2026-03-02', count: 4 },
          { date: '2026-02-28', count: 99 },
        ],
      },
      new Date(2026, 2, 1),
      new Date(2026, 2, 8)
    );
    expect(result?.calendarDays).toBe(8);
    expect(result?.activeDays).toBe(2);
    expect(result?.detections).toBe(24);
    expect(result?.dailyAverage).toBe(3);
    expect(result?.busiest?.count).toBe(14);
    expect(result?.weekdays[6]).toEqual({ day: 6, count: 14, calendarDays: 2, average: 7 });
    expect(result?.weekdays[0]).toEqual({ day: 0, count: 10, calendarDays: 1, average: 10 });
  });

  it('distinguishes missing data from a valid empty period', () => {
    const start = new Date(2026, 10, 9);
    expect(summarizeActivity(null, start, start)).toBeNull();
    const empty = summarizeActivity({ data: [] }, start, start);
    expect(empty?.dailyAverage).toBe(0);
    expect(empty?.activeDays).toBe(0);
    expect(empty?.busiest).toBeNull();
    expect(empty?.weekdays.every(day => day.average === 0)).toBe(true);
    expect(
      summarizeActivity({ data: [{ date: '2026-11-09', count: -1 }] }, start, start)
    ).toBeNull();
  });

  it('resolves busiest-day ties chronologically and rejects reversed periods', () => {
    const start = new Date(2026, 10, 1);
    const end = new Date(2026, 10, 3);
    const result = summarizeActivity(
      {
        data: [
          { date: '2026-11-03', count: 10 },
          { date: '2026-11-01', count: 10 },
        ],
      },
      start,
      end
    );
    expect(result?.busiest?.date.getDate()).toBe(1);
    expect(summarizeActivity({ data: [] }, end, start)).toBeNull();
  });
});
