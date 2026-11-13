<script lang="ts" module>
  export type VisualObservation = {
    id: number;
    camera: string;
    commonName?: string | null;
    originalLabel?: string | null;
    scientificName?: string | null;
    startTime: string;
    detectionConfidence?: number | null;
    classificationConfidence?: number | null;
    snapshotStatus: string;
  };
</script>

<script lang="ts">
  import Card from '$lib/desktop/components/ui/Card.svelte';
  import DataTable from '$lib/desktop/components/data/DataTable.svelte';
  import type { Column } from '$lib/desktop/components/data/DataTable.types';
  import { t } from '$lib/i18n';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  const ObservationTable = DataTable<VisualObservation>;
  interface Props {
    data: VisualObservation[];
    loading?: boolean;
    error?: string;
  }
  let { data, loading = false, error = '' }: Props = $props();
  let failedImages = $state<number[]>([]);
  const PERCENT = 100;
  const columns: Column<VisualObservation>[] = $derived([
    { key: 'snapshotStatus', header: t('cameraLive.snapshot') },
    { key: 'startTime', header: t('cameraLive.observedAt') },
    { key: 'commonName', header: t('cameraLive.species') },
    { key: 'camera', header: t('cameraLive.camera') },
    { key: 'detectionConfidence', header: t('cameraLive.objectConfidence') },
    { key: 'classificationConfidence', header: t('cameraLive.speciesConfidence') },
  ]);
  function confidence(value: number | null | undefined): string {
    return value == null ? t('common.unknown') : `${Math.round(value * PERCENT)}%`;
  }
</script>

<Card title={t('cameraLive.recentObservations')}>
  <ObservationTable
    {columns}
    {data}
    {loading}
    error={error || null}
    emptyMessage={t('cameraLive.noObservations')}
  >
    {#snippet renderCell({ column, item })}
      {#if column.key === 'startTime'}
        <time datetime={item.startTime}>{new Date(item.startTime).toLocaleString()}</time>
      {:else if column.key === 'commonName'}
        <span>{item.commonName || item.originalLabel || t('common.unknown')}</span>
        {#if item.scientificName}<span class="block text-xs italic opacity-70"
            >{item.scientificName}</span
          >{/if}
      {:else if column.key === 'camera'}
        <span class="block text-xs opacity-70">Frigate</span>{item.camera}
      {:else if column.key === 'detectionConfidence'}{confidence(item.detectionConfidence)}
      {:else if column.key === 'classificationConfidence'}{confidence(
          item.classificationConfidence
        )}
      {:else if item.snapshotStatus === 'available'}
        <a
          class="inline-block underline"
          href={buildAppUrl(`/api/v2/visual-detections/${item.id}/snapshot`)}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={t('cameraLive.snapshot')}
        >
          {#if failedImages.includes(item.id)}{t('cameraLive.snapshot')}
          {:else}<img
              src={buildAppUrl(`/api/v2/visual-detections/${item.id}/snapshot`)}
              alt={item.commonName || item.originalLabel || t('cameraLive.snapshot')}
              loading="lazy"
              width="128"
              height="72"
              class="h-18 w-32 max-w-none rounded object-cover"
              onerror={() => {
                failedImages = [...failedImages, item.id];
              }}
            />{/if}
        </a>
      {:else}{t('common.unknown')}{/if}
    {/snippet}
  </ObservationTable>
</Card>
