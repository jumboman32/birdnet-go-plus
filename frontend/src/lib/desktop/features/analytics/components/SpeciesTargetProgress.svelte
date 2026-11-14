<script lang="ts">
  import { t } from '$lib/i18n';
  import { formatNumber } from '$lib/utils/formatters';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { getStoredValue, setStoredValue } from '$lib/utils/storage';
  import { generateId } from '$lib/utils/uuid';
  import { getLogger } from '$lib/utils/logger';

  interface Progress {
    region: string;
    detected: number;
    total: number;
    percentage: number | null;
  }
  interface Response {
    status: 'available' | 'region_required' | 'ebird_required' | 'database_required';
    county?: Progress;
    state?: Progress;
  }
  const logger = getLogger('species-target-progress');
  const STORAGE_KEY = 'analytics.species.targetCounty';
  const fieldId = generateId('target-county');
  const restoredCounty = getStoredValue<string>(
    STORAGE_KEY,
    '',
    value => typeof value === 'string'
  );
  let county = $state(restoredCounty);
  let appliedCounty = $state(restoredCounty);
  const COUNTY_CODE_PATTERN = /^[A-Z]{2}-[A-Za-z0-9]{1,12}-[A-Za-z0-9]{1,12}$/;
  let countyTouched = $state(false);
  const invalidCounty = $derived(county.trim() !== '' && !COUNTY_CODE_PATTERN.test(county.trim()));
  let response = $state<Response | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let retry = $state(0);

  function isProgress(value: unknown): value is Progress {
    if (!value || typeof value !== 'object') return false;
    return (
      'region' in value &&
      typeof value.region === 'string' &&
      'detected' in value &&
      typeof value.detected === 'number' &&
      Number.isSafeInteger(value.detected) &&
      value.detected >= 0 &&
      'total' in value &&
      typeof value.total === 'number' &&
      Number.isSafeInteger(value.total) &&
      value.total >= value.detected &&
      'percentage' in value &&
      (value.percentage === null ||
        (typeof value.percentage === 'number' &&
          Number.isFinite(value.percentage) &&
          value.percentage >= 0 &&
          value.percentage <= 100))
    );
  }

  function isResponse(value: unknown): value is Response {
    if (!value || typeof value !== 'object' || !('status' in value)) return false;
    if (value.status === 'available')
      return (
        'county' in value && isProgress(value.county) && 'state' in value && isProgress(value.state)
      );
    return (
      value.status === 'region_required' ||
      value.status === 'ebird_required' ||
      value.status === 'database_required'
    );
  }

  $effect(() => {
    const search = new URLSearchParams();
    if (appliedCounty) search.set('county', appliedCounty);
    void retry;
    const controller = new AbortController();
    loading = true;
    failed = false;
    response = null;
    void fetch(buildAppUrl(`/api/v2/species/target-progress?${search}`), {
      signal: controller.signal,
    })
      .then(async result => {
        if (!result.ok) throw new Error(`Target progress HTTP ${result.status}`);
        const data: unknown = await result.json();
        if (!isResponse(data)) throw new Error('Invalid target progress response');
        if (!controller.signal.aborted) response = data;
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          logger.error('Failed to load regional species progress', error);
          failed = true;
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false;
      });
    return () => controller.abort();
  });
</script>

<section
  class="rounded-xl border border-[var(--color-base-200)] bg-[var(--color-base-100)] p-4 md:p-6 space-y-4"
  aria-busy={loading}
>
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h2 class="text-lg font-semibold">{t('analytics.species.targets.title')}</h2>
      <p class="text-sm opacity-70">{t('analytics.species.targets.description')}</p>
    </div>
    <form
      novalidate
      class="flex flex-wrap items-end gap-2"
      onsubmit={event => {
        event.preventDefault();
        countyTouched = true;
        if (invalidCounty) return;
        appliedCounty = county.trim();
        setStoredValue(STORAGE_KEY, appliedCounty);
        retry++;
      }}
    >
      <div>
        <label for={fieldId} class="block text-sm mb-1"
          >{t('analytics.species.targets.region')}</label
        ><input
          id={fieldId}
          bind:value={county}
          oninput={() => (countyTouched = false)}
          onblur={() => (countyTouched = true)}
          aria-invalid={countyTouched && invalidCounty ? 'true' : undefined}
          pattern={COUNTY_CODE_PATTERN.source}
          aria-describedby={`${fieldId}-hint${countyTouched && invalidCounty ? ` ${fieldId}-error` : ''}`}
          class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-100)] px-3 py-2 text-sm"
        />
      </div>
      <button
        type="submit"
        class="rounded-lg bg-[var(--color-primary)] px-3 py-2 text-sm text-[var(--color-primary-content)]"
        >{t('common.buttons.apply')}</button
      >
    </form>
  </div>
  <p id={`${fieldId}-hint`} class="text-xs opacity-70">
    {t('analytics.species.targets.regionHint')}
  </p>
  {#if countyTouched && invalidCounty}<p
      id={`${fieldId}-error`}
      class="text-sm text-[var(--text-error)]"
    >
      {t('common.validation.pattern')}
    </p>{/if}
  <div role="status" class="sr-only">{loading ? t('common.ui.loading') : ''}</div>
  {#if loading}<p class="text-sm">{t('common.ui.loading')}</p>
  {:else if failed}<p role="alert" class="text-sm">{t('analytics.errors.loadFailed')}</p>
  {:else if response?.status === 'region_required'}<p class="text-sm">
      {t('analytics.species.targets.regionRequired')}
    </p>
  {:else if response?.status === 'ebird_required'}<p class="text-sm">
      {t('analytics.species.targets.ebirdRequired')}
    </p>
  {:else if response?.status === 'database_required'}<p class="text-sm">
      {t('errors.insights.v2Required')}
    </p>
  {:else if response?.status === 'available'}
    <div class="grid grid-cols-2 gap-4">
      {#each [{ kind: 'county', title: t('analytics.species.targets.county'), progress: response.county }, { kind: 'state', title: t('analytics.species.targets.state'), progress: response.state }] as item (item.kind)}
        {#if item.progress}
          <div class="rounded-lg bg-[var(--color-base-200)]/50 p-4">
            <h3 class="font-medium">
              {item.title} <span class="text-sm opacity-60">{item.progress.region}</span>
            </h3>
            <p class="mt-2 text-2xl font-semibold tabular-nums">
              {formatNumber(item.progress.detected)} / {formatNumber(item.progress.total)}
              <span class="text-base font-normal opacity-70"
                >({item.progress.percentage === null
                  ? '—'
                  : `${item.progress.percentage.toFixed(1)}%`})</span
              >
            </p>
            <p class="text-xs mt-2 opacity-70">
              {item.progress.total === 0
                ? t('common.ui.noData')
                : t('analytics.species.targets.lifetime')}
            </p>
          </div>
        {/if}
      {/each}
    </div>
  {/if}
</section>
