import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../../../../../test/render-helpers';
import PieChart from './PieChart.svelte';

afterEach(cleanup);
const factory = createComponentTestFactory(PieChart);

describe('PieChart', () => {
  it('renders proportionate slices with an aggregate category and supports keyboard navigation', async () => {
    const onSliceClick = vi.fn();
    const view = factory.render({
      data: [
        { label: 'Bird A', value: 30 },
        { label: 'Bird B', value: 50 },
        { label: 'Other species', value: 20, clickable: false },
      ],
      ariaLabel: 'Species pie chart',
      valueTooltipLabel: 'Detections',
      onSliceClick,
    });
    await waitFor(() => expect(view.container.querySelectorAll('.pie-slice')).toHaveLength(3));
    expect(view.container.querySelectorAll('.pie-leader')).toHaveLength(3);
    expect(view.container.querySelectorAll('.pie-label')).toHaveLength(3);
    const first = view.getByRole('button', { name: 'Bird A: 30 (30.0%)' });
    await fireEvent.keyDown(first, { key: 'Enter' });
    expect(onSliceClick).toHaveBeenCalledWith(
      expect.objectContaining({ label: 'Bird A', value: 30 })
    );
    await fireEvent.click(view.getByRole('img', { name: 'Other species: 20 (20.0%)' }));
    expect(onSliceClick).toHaveBeenCalledTimes(1);
    expect(view.getByText('50.0%')).toBeInTheDocument();
  });

  it('omits invalid and zero-valued slices without inventing a distribution', async () => {
    const view = factory.render({
      data: [
        { label: 'Empty', value: 0 },
        { label: 'Invalid', value: Number.NaN },
        { label: 'Negative', value: -1 },
      ],
      ariaLabel: 'Empty pie chart',
      valueTooltipLabel: 'Detections',
    });
    await Promise.resolve();
    expect(view.container.querySelectorAll('.pie-slice')).toHaveLength(0);
    expect(view.container.textContent).not.toContain('NaN');
  });
});
