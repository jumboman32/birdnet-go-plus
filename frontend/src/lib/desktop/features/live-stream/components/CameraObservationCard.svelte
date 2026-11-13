<script lang="ts">
  import { Camera } from '@lucide/svelte';
  import type { VisualObservation } from './CameraObservationResults.svelte';
  import { t } from '$lib/i18n';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  interface Props {
    observation: VisualObservation;
  }
  let { observation }: Props = $props();
  let imageFailed = $state(false);
  const PERCENT = 100;
  const snapshotURL = $derived(buildAppUrl(`/api/v2/visual-detections/${observation.id}/snapshot`));
  const name = $derived(observation.commonName || observation.originalLabel || t('common.unknown'));
</script>

<article
  class="detection-card flex min-w-0 flex-col rounded-xl bg-[var(--color-base-100)] shadow-sm"
>
  <a
    href={snapshotURL}
    target="_blank"
    rel="noopener noreferrer"
    class="relative block min-h-64 flex-1 overflow-hidden rounded-xl bg-[var(--color-base-200)]"
    aria-label={`${t('cameraLive.snapshot')}: ${name}`}
  >
    {#if observation.snapshotStatus === 'available' && !imageFailed}
      <img
        src={snapshotURL}
        alt={name}
        loading="lazy"
        class="absolute inset-0 h-full w-full object-cover"
        onerror={() => {
          imageFailed = true;
        }}
      />
    {:else}<span class="absolute inset-0 flex items-center justify-center"
        ><Camera size={36} /></span
      >{/if}
    <span class="absolute left-2 top-2 rounded bg-black/70 px-2 py-1 text-xs text-white"
      >Frigate · {observation.camera}</span
    >
    <div
      class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 to-transparent p-3 pt-8 text-white"
    >
      <h3 class="text-xl font-semibold">{name}</h3>
      {#if observation.scientificName}<p class="text-xs italic">
          {observation.scientificName}
        </p>{/if}
    </div>
  </a>
  <div class="mt-1.5 h-20 shrink-0 space-y-1 rounded-xl p-2">
    <time class="block text-xs opacity-70" datetime={observation.startTime}
      >{new Date(observation.startTime).toLocaleString()}</time
    >
    <dl class="space-y-1 text-xs">
      <div class="flex justify-between gap-2">
        <dt>{t('cameraLive.objectConfidence')}</dt>
        <dd>
          {observation.detectionConfidence == null
            ? t('common.unknown')
            : `${Math.round(observation.detectionConfidence * PERCENT)}%`}
        </dd>
      </div>
      <div class="flex justify-between gap-2">
        <dt>{t('cameraLive.speciesConfidence')}</dt>
        <dd>
          {observation.classificationConfidence == null
            ? t('common.unknown')
            : `${Math.round(observation.classificationConfidence * PERCENT)}%`}
        </dd>
      </div>
    </dl>
  </div>
</article>
