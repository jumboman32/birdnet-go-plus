/** Available gallery sizes; four detections keep the photographs prominent. */
export const DETECTION_CARD_LIMITS = [1, 2, 4];

/** Default and maximum number of detections visible in the gallery. */
export const MAX_DETECTION_CARDS = 4;

/** Map legacy or invalid display preferences to the four-card gallery. */
export function normalizeDetectionCardLimit(value: unknown): number {
  return typeof value === 'number' && DETECTION_CARD_LIMITS.includes(value)
    ? value
    : MAX_DETECTION_CARDS;
}
