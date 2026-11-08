<!--
  SpeciesInfoBar.svelte

  Bottom info bar displaying species information on detection cards.
  Features a full-card species photograph, name, verification status, and time info.

  Props:
  - detection: Detection - The detection data to display
  - className?: string - Additional CSS classes
-->
<script lang="ts">
  import type { Detection } from '$lib/types/detection.types';
  import ConfidenceBadge from './ConfidenceBadge.svelte';
  import {
    getSpeciesImageUrl,
    handleBirdImageError,
  } from '$lib/desktop/components/ui/image-utils.js';
  import { formatRelativeTime } from '$lib/utils/formatters';
  import { Bird, Check, CircleHelp } from '@lucide/svelte';
  import { cn } from '$lib/utils/cn';
  import { t } from '$lib/i18n';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { navigation } from '$lib/stores/navigation.svelte';
  import { localizeSpeciesName } from '$lib/utils/speciesDisplay';

  interface Props {
    detection: Detection;
    className?: string;
  }

  let { detection, className = '' }: Props = $props();

  // Use the server timestamp (RFC3339 with timezone) for accurate relative time.
  // Without a timezone-aware timestamp, relative time cannot be computed correctly
  // across different timezones, so we return null to skip the display.
  const detectionDateTime = $derived.by(() => {
    if (detection.timestamp) {
      const parsed = new Date(detection.timestamp);
      if (!isNaN(parsed.getTime())) return parsed;
    }
    return null;
  });
  const relativeTime = $derived(formatRelativeTime(detectionDateTime));

  // Check verification status (API returns verified directly on detection)
  const isVerified = $derived(detection.verified === 'correct');
  const isFalsePositive = $derived(detection.verified === 'false_positive');
  const isUnlikely = $derived(detection.unlikely === true);

  // Species photo URL: buildAppUrl prepends the configured base path so the
  // image resolves correctly behind reverse proxies (e.g. /birdnet/...).
  const thumbnailUrl = $derived(getSpeciesImageUrl(detection.scientificName));

  // Localized common name for display in the visitor's UI locale. Falls back to
  // the server-provided common name, then the scientific name.
  const displayName = $derived(localizeSpeciesName(detection.scientificName, detection.commonName));
</script>

<div class={cn('species-info-bar', className)}>
  <!-- Existing species-image API and retrying silhouette fallback. -->
  <div class="species-thumbnail">
    <Bird class="photo-placeholder" aria-hidden="true" strokeWidth={1} />
    {#key thumbnailUrl}
      <img
        src={thumbnailUrl}
        alt={displayName}
        class="thumbnail-image"
        decoding="async"
        loading="lazy"
        onerror={handleBirdImageError}
      />
    {/key}
  </div>

  <!-- Species Info (flex-1) -->
  <div class="species-details">
    <!-- Name row with verification badge -->
    <div class="species-name-row">
      <a
        class="species-name"
        href={buildAppUrl(`/ui/detections/${detection.id}`)}
        onclick={event => {
          if (
            event.button !== 0 ||
            event.metaKey ||
            event.ctrlKey ||
            event.shiftKey ||
            event.altKey
          )
            return;
          event.preventDefault();
          navigation.navigate(`/ui/detections/${detection.id}`);
        }}>{displayName}</a
      >
      {#if isVerified}
        <span
          class="verified-badge"
          title={t('dashboard.recentDetections.status.verified')}
          aria-label={t('dashboard.recentDetections.status.verified')}
        >
          <Check class="size-3" />
        </span>
      {:else if isFalsePositive}
        <span
          class="false-positive-badge"
          role="status"
          aria-label={t('dashboard.recentDetections.status.false')}
          >{t('dashboard.recentDetections.status.false')}</span
        >
      {:else}
        <span
          class="unverified-badge"
          role="status"
          aria-label={t('dashboard.recentDetections.status.unverified')}
          >{t('dashboard.recentDetections.status.unverified')}</span
        >
      {/if}
      {#if isUnlikely}
        <span
          class="unlikely-badge"
          role="status"
          aria-label={t('dashboard.recentDetections.status.unlikely')}
          ><CircleHelp class="size-2.5" />{t('dashboard.recentDetections.status.unlikely')}</span
        >
      {/if}
    </div>

    <!-- Scientific name -->
    <div class="scientific-name">{detection.scientificName}</div>
  </div>

  <!-- Detection time and confidence stay together above the photograph. -->
  <div class="time-info">
    <span class="detection-time">{detection.time}</span>
    <span class="relative-time">{relativeTime}</span>
    <ConfidenceBadge confidence={detection.confidence} />
  </div>
</div>

<style>
  .species-info-bar {
    position: absolute;
    inset: 0;
    z-index: 1;
    color: white;
    background: linear-gradient(145deg, var(--color-base-200), var(--color-base-300));
  }

  .species-info-bar::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(to bottom, rgb(0 0 0 / 0.2), transparent 35%, rgb(0 0 0 / 0.85));
    pointer-events: none;
  }

  .species-thumbnail {
    position: absolute;
    inset: 0;
  }

  .species-thumbnail :global(.photo-placeholder) {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 45%;
    height: 45%;
    transform: translate(-50%, -50%);
    color: var(--color-base-content);
    opacity: 0.2;
  }

  .thumbnail-image {
    /* Preserve more headroom when wide cards crop the species photograph. */
    --species-photo-vertical-focus: 25%;

    position: relative;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center var(--species-photo-vertical-focus);
    transition: transform 400ms ease;
  }

  .thumbnail-image[src*='bird-placeholder.svg'] {
    opacity: 0;
  }

  @media (hover: hover) and (prefers-reduced-motion: no-preference) {
    :global(.detection-card:hover) .thumbnail-image {
      transform: scale(1.03);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .thumbnail-image {
      transition: none;
    }
  }

  /* Species details */
  .species-details {
    position: absolute;
    bottom: 1rem;
    left: 1rem;
    right: 1rem;
    z-index: 2;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }

  .species-name-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .species-name {
    font-weight: 750;
    font-size: clamp(1.5rem, 2.5vw, 2rem);
    text-decoration: none;
    overflow-wrap: anywhere;
    color: white;
    line-height: 1.3;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }

  .species-name:focus-visible {
    outline: 2px solid white;
    outline-offset: 3px;
    border-radius: 0.25rem;
  }

  .verified-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1rem;
    height: 1rem;
    border-radius: 9999px;
    background-color: var(--color-success);
    color: var(--color-success-content);
  }

  .false-positive-badge {
    display: inline-flex;
    align-items: center;
    padding: 0 0.375rem;
    height: 1rem;
    border-radius: 9999px;
    background-color: color-mix(in srgb, var(--color-error) 90%, transparent);
    color: var(--color-error-content);
    font-size: 0.625rem;
    font-weight: 500;
  }

  .unverified-badge {
    display: inline-flex;
    align-items: center;
    padding: 0 0.375rem;
    height: 1rem;
    border-radius: 9999px;
    background-color: rgb(255 255 255 / 0.12);
    color: white;
    font-size: 0.625rem;
    font-weight: 500;
  }

  .unlikely-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.125rem;
    padding: 0 0.375rem;
    height: 1rem;
    border-radius: 9999px;
    background-color: color-mix(in srgb, var(--color-warning) 90%, transparent);
    color: var(--color-warning-content);
    font-size: 0.625rem;
    font-weight: 500;
  }

  .scientific-name {
    font-size: 0.8125rem;
    font-style: italic;
    color: rgb(255 255 255 / 0.9);
    line-height: 1.3;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }

  /* Time info */
  .time-info {
    position: absolute;
    top: 0.75rem;
    left: 0.75rem;
    max-width: calc(100% - 6.5rem);
    z-index: 2;
    padding: 0.25rem 0.5rem;
    border-radius: 0.5rem;
    background: rgb(0 0 0 / 0.65);
    display: flex;
    flex-flow: row nowrap;
    align-items: center;
    gap: 0.5rem;
  }

  .detection-time {
    flex-shrink: 0;
    font-size: 0.875rem;
    font-weight: 500;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    color: white;
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }

  .relative-time {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.75rem;
    color: rgb(255 255 255 / 0.9);
    text-shadow: 0 1px 2px rgb(0 0 0 / 0.5);
  }
</style>
