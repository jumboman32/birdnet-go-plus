import { describe, it, expect, beforeEach, vi } from 'vitest';
import { fireEvent, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { renderTyped } from '../../../../../test/render-helpers';
import FrigateSettings from './FrigateSettings.svelte';
import { DEFAULT_FRIGATE, settingsStore } from '$lib/stores/settings';
import { api } from '$lib/utils/api';

describe('Frigate settings', () => {
  beforeEach(() => {
    settingsStore.update(state => {
      const data = {
        ...state.formData,
        realtime: {
          ...state.formData.realtime,
          frigate: { ...DEFAULT_FRIGATE, baseURL: 'http://frigate:5000' },
        },
      };
      return {
        ...state,
        originalData: globalThis.structuredClone(data),
        formData: globalThis.structuredClone(data),
      };
    });
  });

  it('updates the real settings path and requires saving before testing', async () => {
    renderTyped(FrigateSettings);
    await fireEvent.input(screen.getByLabelText('settings.integration.frigate.baseURL'), {
      target: { value: 'http://camera-nvr:5000' },
    });
    await fireEvent.change(screen.getByLabelText('settings.integration.frigate.baseURL'));
    expect(get(settingsStore).formData.realtime?.frigate?.baseURL).toBe('http://camera-nvr:5000');
    expect(
      screen.getByRole('button', { name: 'settings.integration.frigate.test' })
    ).toHaveAttribute('aria-disabled', 'true');
  });

  it('tests the saved connection through the CSRF-aware API helper', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ success: true });
    renderTyped(FrigateSettings);
    await fireEvent.click(
      screen.getByRole('button', { name: 'settings.integration.frigate.test' })
    );
    expect(post).toHaveBeenCalledWith('/api/v2/integrations/frigate/test');
    expect(await screen.findByText('settings.integration.frigate.connected')).toBeInTheDocument();
    post.mockRestore();
  });
});
