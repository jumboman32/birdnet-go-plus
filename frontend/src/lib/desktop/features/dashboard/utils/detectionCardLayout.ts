/** Available gallery sizes for up to two rows of three photographs. */
export const DETECTION_CARD_LIMITS = [1, 2, 3, 6];

/** Default and maximum number of detections visible in the gallery. */
export const MAX_DETECTION_CARDS = 6;

/** Map legacy or invalid display preferences to the six-card gallery. */
export function normalizeDetectionCardLimit(value: unknown): number {
  return typeof value === 'number' && DETECTION_CARD_LIMITS.includes(value)
    ? value
    : MAX_DETECTION_CARDS;
}
