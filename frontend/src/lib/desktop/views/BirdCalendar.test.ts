import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../test/render-helpers';
import { ApiError, fetchWithCSRF } from '$lib/utils/api';
import { calendarFixture } from '../../../test/birdCalendarFixtures';
import BirdCalendar from './BirdCalendar.svelte';

vi.mock('$lib/utils/api', async importOriginal => ({
  ...(await importOriginal<typeof import('$lib/utils/api')>()),
  fetchWithCSRF: vi.fn(),
}));
const factory = createComponentTestFactory(BirdCalendar);

describe('native bird calendar', () => {
  beforeEach(() => {
    vi.mocked(fetchWithCSRF).mockReset();
    vi.mocked(fetchWithCSRF).mockResolvedValue(calendarFixture());
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('refreshes automatically and stops polling when leaving the page', async () => {
    vi.useFakeTimers();
    factory.render({});
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchWithCSRF).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchWithCSRF).toHaveBeenCalledTimes(2);
    cleanup();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchWithCSRF).toHaveBeenCalledTimes(2);
  });

  it('loads the private report and supports species search', async () => {
    const view = factory.render({});
    await waitFor(() =>
      expect(view.getByRole('rowheader', { name: /Blue Jay/ })).toBeInTheDocument()
    );
    expect(fetchWithCSRF).toHaveBeenCalledWith(
      '/api/v2/species/calendar',
      expect.objectContaining({ cache: 'no-store' })
    );
    await fireEvent.input(view.getByRole('searchbox'), {
      target: { value: 'not a matching species' },
    });
    expect(view.queryByRole('rowheader', { name: /Blue Jay/ })).not.toBeInTheDocument();
    await fireEvent.input(view.getByRole('searchbox'), {
      target: { value: 'Cyanocitta cristata' },
    });
    expect(view.getByRole('rowheader', { name: /Blue Jay/ })).toBeInTheDocument();
  });

  it('explains a missing import instead of rendering a broken calendar', async () => {
    vi.mocked(fetchWithCSRF).mockRejectedValue(
      new ApiError('Not found', 404, new Response('', { status: 404 }))
    );
    const view = factory.render({});
    await waitFor(() => expect(view.getByText('calendar.missingTitle')).toBeInTheDocument());
    expect(view.queryByRole('table')).not.toBeInTheDocument();
  });

  it('aborts the request when leaving the page', async () => {
    vi.mocked(fetchWithCSRF).mockReturnValue(new Promise(() => {}));
    factory.render({});
    await waitFor(() => expect(fetchWithCSRF).toHaveBeenCalled());
    const options = vi.mocked(fetchWithCSRF).mock.calls.at(0)?.[1];
    cleanup();
    expect(options?.signal?.aborted).toBe(true);
  });
});
