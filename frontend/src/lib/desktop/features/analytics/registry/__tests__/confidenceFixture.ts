import type { ConfidenceSeries } from '../../utils/confidenceAnalytics';

export function confidenceFixture(overrides: Partial<ConfidenceSeries> = {}): ConfidenceSeries {
  return {
    modelId: 1,
    model: 'BirdNET',
    species: '',
    count: 4,
    bins: Array.from({ length: 20 }, (_, index) =>
      index === 4 || index === 5 || index === 16 || index === 19 ? 1 : 0
    ),
    average: 0.5625,
    median: 0.525,
    low: 2,
    middle: 0,
    high: 2,
    uniqueSpecies: 1,
    topSpecies: 'Turdus merula',
    topCount: 4,
    ...overrides,
  };
}
