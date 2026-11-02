<script lang="ts">
  import { t } from '$lib/i18n';
  import Button from '$lib/desktop/components/ui/Button.svelte';
  import { buildAppUrl } from '$lib/utils/urlHelpers';

  interface Props {
    scientificName: string;
    date: string;
  }
  interface RarityData {
    score: number | null;
    scoreLow: number | null;
    scoreHigh: number | null;
    reason: string;
    reasonCode: string;
    samples: number;
    reports: number;
    years: number;
    regionName: string;
    radiusKm: number;
    source: string;
    retrievedAt: string;
    recentStatus: string;
    recentReported: boolean;
    birdcastUrl: string;
  }

  const { scientificName, date }: Props = $props();
  let data = $state<RarityData | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let retry = $state(0);

  function validScore(value: unknown): value is number | null {
    return (
      value === null ||
      (typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 100)
    );
  }

  function parseData(value: unknown): RarityData | null {
    if (
      typeof value !== 'object' ||
      value === null ||
      !('status' in value) ||
      (value.status !== 'available' && value.status !== 'insufficient_data') ||
      !('score' in value) ||
      !validScore(value.score) ||
      !('scoreLow' in value) ||
      !validScore(value.scoreLow) ||
      !('scoreHigh' in value) ||
      !validScore(value.scoreHigh) ||
      !('reason' in value) ||
      typeof value.reason !== 'string' ||
      !('samples' in value) ||
      typeof value.samples !== 'number' ||
      !('reports' in value) ||
      typeof value.reports !== 'number' ||
      !('years' in value) ||
      typeof value.years !== 'number' ||
      !('regionName' in value) ||
      typeof value.regionName !== 'string' ||
      !('radiusKm' in value) ||
      typeof value.radiusKm !== 'number' ||
      !('source' in value) ||
      typeof value.source !== 'string' ||
      !('retrievedAt' in value) ||
      typeof value.retrievedAt !== 'string' ||
      !('recentStatus' in value) ||
      typeof value.recentStatus !== 'string' ||
      !('recentReported' in value) ||
      typeof value.recentReported !== 'boolean' ||
      !('birdcastUrl' in value) ||
      typeof value.birdcastUrl !== 'string'
    )
      return null;
    if (
      value.status === 'available' &&
      (value.score === null || value.scoreLow === null || value.scoreHigh === null)
    )
      return null;
    return {
      score: value.status === 'available' ? value.score : null,
      scoreLow: value.scoreLow,
      scoreHigh: value.scoreHigh,
      reason: value.reason,
      reasonCode:
        'reasonCode' in value && typeof value.reasonCode === 'string'
          ? value.reasonCode
          : 'unavailable',
      samples: value.samples,
      reports: value.reports,
      years: value.years,
      regionName: value.regionName,
      radiusKm: value.radiusKm,
      source: value.source,
      retrievedAt: value.retrievedAt,
      recentStatus: value.recentStatus,
      recentReported: value.recentReported,
      birdcastUrl:
        /^https:\/\/dashboard\.birdcast\.org\/region\/US-[A-Z]{2}-\d{3}\?night=\d{4}-\d{2}-\d{2}$/.test(
          value.birdcastUrl
        )
          ? value.birdcastUrl
          : '',
    };
  }

  function reasonText(code: string): string {
    switch (code) {
      case 'missing_data':
        return t('species.localRarity.reasons.missingData');
      case 'station_mismatch':
        return t('species.localRarity.reasons.stationMismatch');
      case 'unmapped_taxon':
        return t('species.localRarity.reasons.unmappedTaxon');
      case 'insufficient_samples':
        return t('species.localRarity.reasons.insufficientSamples');
      case 'location_required':
        return t('species.localRarity.reasons.locationRequired');
      case 'invalid_import':
        return t('species.localRarity.reasons.invalidImport');
      default:
        return t('species.localRarity.reasons.unavailable');
    }
  }

  $effect(() => {
    const params = new URLSearchParams({ scientific_name: scientificName, date });
    const controller = new AbortController();
    void retry;
    data = null;
    loading = true;
    failed = false;
    async function load() {
      try {
        const response = await fetch(buildAppUrl(`/api/v2/species/local-rarity?${params}`), {
          signal: controller.signal,
        });
        if (!response.ok) throw new Error('Rarity request failed');
        const parsed = parseData(await response.json());
        if (!parsed) throw new Error('Invalid rarity response');
        if (!controller.signal.aborted) data = parsed;
      } catch {
        if (!controller.signal.aborted) failed = true;
      } finally {
        if (!controller.signal.aborted) loading = false;
      }
    }
    void load();
    return () => controller.abort();
  });
</script>

<section
  aria-label={t('species.localRarity.title')}
  class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-100)] p-5 space-y-3"
>
  <h3 class="font-semibold">{t('species.localRarity.title')}</h3>
  <div aria-live="polite" aria-busy={loading}>
    {#if loading}
      <p>{t('species.localRarity.loading')}</p>
    {:else if failed}
      <p>{t('species.localRarity.failed')}</p>
      <Button type="button" size="sm" className="mt-2" onclick={() => retry++}>
        {t('species.localRarity.retry')}
      </Button>
    {:else if data}
      {#if data.score !== null}
        <p class="text-3xl font-semibold">
          {data.score}<span class="text-base font-normal"> / 100</span>
        </p>
        <p class="text-sm">{t('species.localRarity.interpretation')}</p>
      {:else}
        <p class="font-medium">{t('species.localRarity.insufficient')}</p>
        <p class="text-sm">{reasonText(data.reasonCode)}</p>
      {/if}
      {#if data.regionName}
        <p class="text-sm">
          {t('species.localRarity.geography', {
            region: data.regionName,
            radius: data.radiusKm,
            date,
          })}
        </p>
      {/if}
      <details class="text-sm">
        <summary class="cursor-pointer font-medium">{t('species.localRarity.evidence')}</summary>
        <div class="mt-2 space-y-2">
          <p>
            {t('species.localRarity.counts', {
              reports: data.reports,
              samples: data.samples,
              years: data.years,
            })}
          </p>
          {#if data.score !== null && data.scoreLow !== null && data.scoreHigh !== null}
            <p>{t('species.localRarity.interval', { low: data.scoreLow, high: data.scoreHigh })}</p>
          {/if}
          <p>{t('species.localRarity.method')}</p>
          {#if data.source}
            <p>
              {t('species.localRarity.source', { source: data.source, date: data.retrievedAt })}
            </p>
          {/if}
          {#if data.recentStatus === 'available'}
            <p>
              {data.recentReported
                ? t('species.localRarity.recentReported')
                : t('species.localRarity.recentMissing')}
              {t('species.localRarity.recentLimit')}
            </p>
          {:else if data.recentStatus === 'historical_unavailable'}
            <p>{t('species.localRarity.historical')}</p>
          {:else}
            <p>{t('species.localRarity.recentUnavailable')}</p>
          {/if}
          <p>{t('species.localRarity.model')}</p>
        </div>
      </details>
      {#if data.birdcastUrl}
        <p class="text-sm">
          <a
            href={data.birdcastUrl}
            target="_blank"
            rel="noopener noreferrer"
            class="underline underline-offset-2 hover:text-[var(--color-primary)]"
            >{t('species.localRarity.birdcastLink')}</a
          >.
          {t('species.localRarity.birdcastContext')}
        </p>
      {/if}
    {/if}
  </div>
</section>
