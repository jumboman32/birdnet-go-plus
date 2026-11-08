import { describe, expect, it } from 'vitest';
import { normalizeDetectionCardLimit } from './detectionCardLayout';

describe('photographic gallery size preferences', () => {
  it.each([1, 2, 3, 6])('preserves the supported %i-card preference', value => {
    expect(normalizeDetectionCardLimit(value)).toBe(value);
  });

  it.each([4, 12, 24, 48, 5, 10, 25, 50, 0, -1, NaN, '4', null, undefined])(
    'defaults legacy or invalid preference %s to six cards',
    value => {
      expect(normalizeDetectionCardLimit(value)).toBe(6);
    }
  );
});
