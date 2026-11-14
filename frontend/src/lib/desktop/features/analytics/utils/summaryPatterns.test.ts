import { describe, expect, it } from 'vitest';
import {
  comparisonWindow,
  peakActivityWindow,
  relativeChange,
  speciesBalance,
  speciesMovements,
} from './summaryPatterns';

describe('Summary pattern calculations', () => {
  it('compares equal calendar periods across DST and excludes the unfinished current day', () => {
    expect(
      comparisonWindow(new Date(2026, 2, 7), new Date(2026, 2, 10), new Date(2026, 2, 10, 8))
    ).toEqual({
      currentStart: '2026-03-07',
      currentEnd: '2026-03-09',
      previousStart: '2026-03-04',
      previousEnd: '2026-03-06',
    });
    expect(
      comparisonWindow(new Date(2026, 2, 10), new Date(2026, 2, 10), new Date(2026, 2, 10, 8))
    ).toBeNull();
    expect(comparisonWindow(new Date('invalid'), new Date(), new Date())).toBeNull();
  });

  it('distinguishes balanced diversity from dominance and combines species across models', () => {
    const balanced = speciesBalance([
      { scientific_name: 'A', common_name: 'A', count: 25 },
      { scientific_name: 'A', common_name: 'A', count: 25 },
      { scientific_name: 'B', common_name: 'B', count: 50 },
    ]);
    expect(balanced.total).toBe(100);
    expect(balanced.species).toHaveLength(2);
    expect(balanced.effective).toBeCloseTo(2);
    expect(balanced.speciesForCoverage).toBe(2);
    const dominant = speciesBalance([
      { scientific_name: 'A', common_name: 'A', count: 90 },
      { scientific_name: 'B', common_name: 'B', count: 10 },
    ]);
    expect(dominant.effective).toBeLessThan(balanced.effective);
    expect(dominant.speciesForCoverage).toBe(1);
    expect(speciesBalance([]).effective).toBe(0);
  });

  it('calculates top-five share from the full species set, ignoring invalid counts', () => {
    const rows = Array.from({ length: 10 }, (_, index) => ({
      scientific_name: `Bird ${index}`,
      common_name: '',
      count: 10,
    }));
    const result = speciesBalance([...rows, { common_name: 'Invalid', count: -1 }]);
    expect(result.topFiveShare).toBe(50);
    expect(result.speciesForCoverage).toBe(8);
    expect(result.effective).toBeCloseTo(10);
  });

  it('ranks share changes independently of the overall number of recordings', () => {
    const before = speciesBalance([
      { common_name: 'A', count: 20 },
      { common_name: 'B', count: 80 },
    ]);
    const after = speciesBalance([
      { common_name: 'A', count: 100 },
      { common_name: 'B', count: 100 },
    ]);
    const movements = speciesMovements(after, before);
    expect(movements.find(row => row.species.key === 'A')?.change).toBe(30);
    expect(movements.find(row => row.species.key === 'B')?.change).toBe(-30);
    expect(speciesMovements(after, speciesBalance([]))).toEqual([]);
    expect(relativeChange(200, 100)).toBe(100);
    expect(relativeChange(0, 100)).toBe(-100);
    expect(relativeChange(10, 0)).toBeNull();
    expect(relativeChange(0, 0)).toBeNull();
  });

  it('includes species entering and leaving the detection mix', () => {
    const before = speciesBalance([{ common_name: 'Old', count: 10 }]);
    const after = speciesBalance([{ common_name: 'New', count: 10 }]);
    const movements = speciesMovements(after, before);
    expect(movements.find(row => row.species.key === 'Old')?.change).toBe(-100);
    expect(movements.find(row => row.species.key === 'New')?.change).toBe(100);
  });

  it('finds activity windows across midnight without interpreting missing detections as downtime', () => {
    expect(
      peakActivityWindow([
        { hour: 23, count: 10 },
        { hour: 0, count: 20 },
        { hour: 1, count: 10 },
      ])
    ).toEqual({ start: 23, end: 2, share: 100 });
    expect(peakActivityWindow([])).toBeNull();
    expect(
      peakActivityWindow([
        { hour: 24, count: 100 },
        { hour: 2, count: -1 },
      ])
    ).toBeNull();
  });
});
