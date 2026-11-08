import { describe, expect, it } from 'vitest';
import { calendarWeekPeriod, isBirdCalendarReport } from './birdCalendar';
import { calendarFixture } from '../../test/birdCalendarFixtures';

describe('bird calendar import boundary', () => {
  it('accepts a complete snapshot and rejects malformed data before rendering', () => {
    const report = calendarFixture();
    expect(isBirdCalendarReport(report)).toBe(true);
    expect(isBirdCalendarReport({ ...report, currentWeek: 52 })).toBe(false);
    expect(isBirdCalendarReport({ ...report, timezone: 'not/a/timezone' })).toBe(false);
    expect(
      isBirdCalendarReport({ ...report, species: [...report.species, ...report.species] })
    ).toBe(false);
    expect(isBirdCalendarReport({ ...report, samples: [] })).toBe(false);
    expect(
      isBirdCalendarReport({
        ...report,
        species: [{ ...report.species[0], rates: Array.from({ length: 52 }, () => 1.5) }],
      })
    ).toBe(false);
  });

  it('includes December 31 in the final seasonal period', () => {
    expect(calendarWeekPeriod(0, 'en-US')).toBe('Jan 1 – Jan 7');
    expect(calendarWeekPeriod(51, 'en-US')).toBe('Dec 24 – Dec 31');
  });
});
