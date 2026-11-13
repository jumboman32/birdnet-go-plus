<script lang="ts">
  import Checkbox from '$lib/desktop/components/forms/Checkbox.svelte';
  import NumberField from '$lib/desktop/components/forms/NumberField.svelte';
  import TextInput from '$lib/desktop/components/forms/TextInput.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import {
    DEFAULT_FRIGATE,
    settingsStore,
    settingsActions,
    type FrigateSettings,
  } from '$lib/stores/settings';
  import { t } from '$lib/i18n';
  import { api } from '$lib/utils/api';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { hasSettingsChanged } from '$lib/utils/settingsChanges';
  import { generateId } from '$lib/utils/uuid';

  const helpId = generateId('frigate-test-help');
  const urlId = generateId('frigate-url');
  const camerasId = generateId('frigate-cameras');
  let config = $derived({ ...DEFAULT_FRIGATE, ...$settingsStore.formData.realtime?.frigate });
  let hasChanges = $derived(
    hasSettingsChanged(
      $settingsStore.originalData.realtime?.frigate,
      $settingsStore.formData.realtime?.frigate
    )
  );
  let testing = $state(false);
  let message = $state('');
  let error = $state('');
  let canTest = $derived(!testing && !hasChanges && Boolean(config.baseURL));
  const fields = [
    { key: 'pollInterval', min: 1, max: 3600, step: 1 },
    { key: 'requestTimeout', min: 1, max: 3600, step: 1 },
    { key: 'maxAttempts', min: 1, max: 8, step: 1 },
    { key: 'initialHistoryHours', min: 0, max: 87600, step: 1 },
    { key: 'overlapSeconds', min: 1, max: 315360000, step: 1 },
    { key: 'classificationThreshold', min: 0, max: 1, step: 0.01 },
    { key: 'correlationWindow', min: 1, max: 3600, step: 1 },
    { key: 'snapshotRetentionHours', min: 1, max: 87600, step: 1 },
  ] satisfies {
    key: keyof Pick<
      FrigateSettings,
      | 'pollInterval'
      | 'requestTimeout'
      | 'maxAttempts'
      | 'initialHistoryHours'
      | 'overlapSeconds'
      | 'classificationThreshold'
      | 'correlationWindow'
      | 'snapshotRetentionHours'
    >;
    min: number;
    max: number;
    step: number;
  }[];

  function update(values: Partial<FrigateSettings>) {
    settingsActions.updateSection('realtime', { frigate: { ...config, ...values } });
    message = '';
    error = '';
  }

  async function testConnection() {
    if (!canTest) return;
    testing = true;
    message = '';
    error = '';
    try {
      await api.post('/api/v2/integrations/frigate/test');
      message = t('settings.integration.frigate.connected');
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('common.error');
    } finally {
      testing = false;
    }
  }
</script>

<SettingsSection title="Frigate" {hasChanges}>
  <div class="space-y-4">
    <p class="text-sm">{t('settings.integration.frigate.help')}</p>
    <Checkbox
      label={t('common.enabled')}
      checked={config.enabled}
      onchange={enabled => update({ enabled })}
    />
    <TextInput
      id={urlId}
      label={t('settings.integration.frigate.baseURL')}
      value={config.baseURL}
      type="url"
      onchange={baseURL => update({ baseURL })}
    />
    <TextInput
      id={camerasId}
      label={t('settings.integration.frigate.cameras')}
      value={config.cameras.join(', ')}
      onchange={value =>
        update({
          cameras: value
            .split(',')
            .map(camera => camera.trim())
            .filter(Boolean),
        })}
    />
    <div class="grid grid-cols-2 gap-4">
      {#each fields as field (field.key)}
        <NumberField
          label={t(`settings.integration.frigate.${field.key}`)}
          value={config[field.key]}
          min={field.min}
          max={field.max}
          step={field.step}
          onUpdate={value => update({ [field.key]: value })}
        />
      {/each}
    </div>
    <Checkbox
      label={t('settings.integration.frigate.snapshotImport')}
      checked={config.snapshotImport}
      onchange={snapshotImport => update({ snapshotImport })}
    />
    <button
      type="button"
      class="px-4 py-2 rounded-lg border border-[var(--border-200)] aria-disabled:opacity-50"
      aria-disabled={!canTest}
      aria-describedby={helpId}
      onclick={testConnection}
    >
      {testing ? t('common.loading') : t('settings.integration.frigate.test')}
    </button>
    <p id={helpId} class="text-sm">{t('settings.integration.frigate.savedTest')}</p>
    <a class="inline-block text-sm underline" href={buildAppUrl('/ui/cameras')}
      >{t('cameraLive.title')}</a
    >
    <p role="status">{message}</p>
    <p role="alert" class="text-[var(--text-error)]">{error}</p>
  </div>
</SettingsSection>
