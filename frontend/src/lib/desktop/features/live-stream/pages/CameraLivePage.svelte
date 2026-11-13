<script lang="ts">
  import { onMount } from 'svelte';
  import { Camera, Pause, Play } from '@lucide/svelte';
  import Card from '$lib/desktop/components/ui/Card.svelte';
  import CameraObservationResults, {
    type VisualObservation,
  } from '../components/CameraObservationResults.svelte';
  import ErrorAlert from '$lib/desktop/components/ui/ErrorAlert.svelte';
  import SelectDropdown from '$lib/desktop/components/forms/SelectDropdown.svelte';
  import { api } from '$lib/utils/api';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { t } from '$lib/i18n';

  interface CameraInfo {
    name: string;
    online: boolean;
    fps: number;
  }
  interface CameraResponse {
    enabled: boolean;
    cameras: CameraInfo[];
  }
  const OBSERVATIONS_API = '/api/v2/visual-detections?limit=20';
  let observations = $state<VisualObservation[]>([]);
  let observationsLoading = $state(true);
  let observationsError = $state('');

  const STATUS_INTERVAL = 10_000;
  const STREAM_INTERVAL = 25_000;
  const READY_INTERVAL = 500;
  const CAMERAS_API = '/api/v2/integrations/frigate/cameras';
  let data = $state<CameraResponse | null>(null);
  let selected = $state('');
  let paused = $state(false);
  let visible = $state(true);
  let error = $state('');
  let streamError = $state(false);
  let ready = $state(false);
  let streamSource = $state('');
  let image: HTMLImageElement | undefined = $state();
  let camera = $derived(data?.cameras.find(item => item.name === selected));
  let active = $derived(Boolean(data?.enabled && camera?.online && !paused && visible && !error));
  let options = $derived(data?.cameras.map(item => ({ value: item.name, label: item.name })) ?? []);
  const settingsURL = buildAppUrl('/ui/settings/integrations?tab=frigate');

  onMount(() => {
    let disposed = false;
    let generation = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let request: AbortController | undefined;
    async function load() {
      clearTimeout(timer);
      const current = ++generation;
      request?.abort();
      request = new AbortController();
      try {
        const [cameras, visuals] = await Promise.allSettled([
          api.get<CameraResponse>(CAMERAS_API, { signal: request.signal }),
          api.get<{ data: VisualObservation[] }>(OBSERVATIONS_API, { signal: request.signal }),
        ]);
        if (disposed || current !== generation) return;
        if (cameras.status === 'fulfilled') {
          data = cameras.value;
          error = '';
          if (!cameras.value.cameras.some(item => item.name === selected))
            selected = cameras.value.cameras[0]?.name ?? '';
        } else {
          error = t('cameraLive.connectionError');
        }
        observationsLoading = false;
        if (visuals.status === 'fulfilled') {
          observations = visuals.value.data;
          observationsError = '';
        } else {
          observationsError = t('cameraLive.observationsError');
        }
      } finally {
        if (!disposed && current === generation && visible)
          timer = setTimeout(load, STATUS_INTERVAL);
      }
    }
    function visibilityChanged() {
      visible = !document.hidden;
      generation++;
      request?.abort();
      clearTimeout(timer);
      if (visible) void load();
    }
    visible = !document.hidden;
    if (visible) void load();
    document.addEventListener('visibilitychange', visibilityChanged);
    return () => {
      disposed = true;
      clearTimeout(timer);
      request?.abort();
      document.removeEventListener('visibilitychange', visibilityChanged);
    };
  });

  $effect(() => {
    const name = selected;
    ready = false;
    streamError = false;
    if (!active || !name) {
      streamSource = '';
      return;
    }
    function connect() {
      streamError = false;
      streamSource =
        buildAppUrl(`${CAMERAS_API}/${encodeURIComponent(name)}/live`) + `?session=${Date.now()}`;
    }
    connect();
    const renew = setInterval(connect, STREAM_INTERVAL);
    // Some browsers report multipart-image load only when the connection ends.
    const check = setInterval(() => {
      if (image?.naturalWidth && !streamError) ready = true;
    }, READY_INTERVAL);
    return () => {
      clearInterval(renew);
      clearInterval(check);
    };
  });
</script>

<svelte:head><title>{t('cameraLive.title')}</title></svelte:head>

<div class="col-span-12 min-w-0 w-full space-y-5">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold flex items-center gap-2">
        <Camera size={26} />{t('cameraLive.title')}
      </h1>
      <p class="mt-1 text-sm opacity-70">{t('cameraLive.description')}</p>
    </div>
    <a class="text-sm underline shrink-0" href={settingsURL}>{t('cameraLive.settings')}</a>
  </div>
  {#if error}<ErrorAlert message={error} />{/if}
  {#if !data && !error}
    <p role="status">{t('common.loading')}</p>
  {:else if data && !data.enabled}
    <Card
      ><p>{t('cameraLive.disabled')}</p>
      <a class="underline" href={settingsURL}>{t('cameraLive.settings')}</a></Card
    >
  {:else if data && data.cameras.length === 0}
    <Card
      ><p>{t('cameraLive.empty')}</p>
      <a class="underline" href={settingsURL}>{t('cameraLive.settings')}</a></Card
    >
  {:else if data}
    <Card>
      <div class="flex items-center justify-between gap-4 mb-4">
        <SelectDropdown
          label={t('cameraLive.camera')}
          value={selected}
          {options}
          onChange={value => {
            if (typeof value === 'string') selected = value;
          }}
        />
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-lg border border-[var(--border-200)] px-4 py-2 text-sm"
          onclick={() => {
            paused = !paused;
          }}
        >
          {#if paused}<Play size={16} />{t('cameraLive.resume')}{:else}<Pause size={16} />{t(
              'cameraLive.pause'
            )}{/if}
        </button>
      </div>
      <div class="relative aspect-video overflow-hidden rounded-lg bg-black">
        {#if active && streamSource}
          <img
            bind:this={image}
            src={streamSource}
            alt={selected}
            class="h-full w-full object-contain"
            onload={() => {
              ready = true;
            }}
            onerror={() => {
              streamError = true;
              ready = false;
            }}
          />
        {/if}
        {#if !camera?.online || paused || !visible || !ready || streamError}
          <div
            class="absolute inset-0 flex items-center justify-center p-6 text-center text-white bg-black/50"
          >
            <div role="status">
              {#if !camera?.online}<p class="text-lg">{t('cameraLive.offline')}</p>
                <p class="mt-2 text-sm">{t('cameraLive.offlineHelp')}</p>
              {:else if paused || !visible}<p>{t('cameraLive.paused')}</p>
              {:else if streamError}<p>{t('cameraLive.unavailable')}</p>
              {:else}<p>{t('cameraLive.connecting')}</p>{/if}
            </div>
          </div>
        {/if}
      </div>
      <div class="mt-3 flex items-center justify-between text-sm">
        <span role="status"
          >{camera?.online && active && ready && !streamError
            ? t('cameraLive.live')
            : paused || !visible
              ? t('cameraLive.paused')
              : !camera?.online
                ? t('cameraLive.offline')
                : streamError
                  ? t('cameraLive.unavailable')
                  : t('cameraLive.connecting')}</span
        >
        <span class="opacity-70">{t('cameraLive.videoOnly')}</span>
      </div>
    </Card>
  {/if}
  <CameraObservationResults
    data={observations}
    loading={observationsLoading}
    error={observationsError}
  />
</div>
