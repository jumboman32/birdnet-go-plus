import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import SpeciesTargetProgress from './SpeciesTargetProgress.svelte';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.removeItem('analytics.species.targetCounty');
});

describe('SpeciesTargetProgress', () => {
  it('shows distinct county and state fractions and percentages', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          status: 'available',
          county: { region: 'US-NJ-001', detected: 5, total: 10, percentage: 50 },
          state: { region: 'US-NJ', detected: 8, total: 40, percentage: 20 },
        }),
      })
    );
    const { container, getByText } = renderTyped(SpeciesTargetProgress);
    await waitFor(() => expect(getByText('(50.0%)')).toBeInTheDocument());
    expect(getByText('(20.0%)')).toBeInTheDocument();
    expect(container.textContent).toContain('5 / 10');
    expect(container.textContent).toContain('8 / 40');
    expect(container.textContent).toContain('analytics.species.targets.lifetime');
  });

  it('Accessibility: explains invalid county input and does not send it to eBird', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ status: 'region_required' }) });
    vi.stubGlobal('fetch', fetch);
    const { container, getByLabelText, getByText } = renderTyped(SpeciesTargetProgress);
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    const input = getByLabelText('analytics.species.targets.region');
    await fireEvent.input(input, { target: { value: 'US-NJ-001/../../' } });
    const form = container.querySelector('form');
    if (!form) throw new Error('Expected county form');
    await fireEvent.submit(form);
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(getByText('common.validation.pattern')).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledOnce();
  });

  it('does not invent percentages for empty lists', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          status: 'available',
          county: { region: 'US-NJ-001', detected: 0, total: 0, percentage: null },
          state: { region: 'US-NJ', detected: 0, total: 0, percentage: null },
        }),
      })
    );
    const { container } = renderTyped(SpeciesTargetProgress);
    await waitFor(() => expect(container.textContent).toContain('0 / 0'));
    expect(container.textContent).not.toContain('NaN');
    expect(container.textContent).not.toContain('Infinity');
    expect(container.textContent).toContain('common.ui.noData');
  });
});
