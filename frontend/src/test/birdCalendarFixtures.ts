import type { BirdCalendarReport } from '$lib/utils/birdCalendar';

/** Build an entirely synthetic private-calendar response for UI tests. */
export function calendarFixture(): BirdCalendarReport {
  return {
    version: 1,
    year: 2026,
    currentWeek: 44,
    generated: '2026-11-07T19:00:00-05:00',
    timezone: 'America/New_York',
    region: 'Synthetic county',
    radius: 25,
    citation: 'Synthetic test data',
    firstHistoricalYear: 2016,
    lastHistoricalYear: 2025,
    species: [
      {
        scientific: 'Cyanocitta cristata',
        name: 'Blue Jay',
        rates: Array.from({ length: 52 }, () => 0.4),
        heard: Array.from({ length: 52 }, (_, week) => (week === 44 ? 1 : 0)),
        counts: Array.from({ length: 52 }, (_, week) => (week === 44 ? 3 : 0)),
        arrivals: [44],
        peaks: [44],
        departures: [46],
        resident: false,
      },
    ],
    samples: Array.from({ length: 52 }, () => 100),
    years: Array.from({ length: 52 }, () => 3),
    stationDays: Array.from({ length: 52 }, (_, week) => (week === 44 ? 1 : 0)),
    firstDetectionDate: '2026-11-07',
    lastDetectionDate: '2026-11-07',
    totalDetections: 3,
  };
}
