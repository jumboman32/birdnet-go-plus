import { afterEach, describe, expect, it } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import { createComponentTestFactory } from '../../../../../../test/render-helpers';
import StatCard from './StatCard.svelte';

afterEach(cleanup);
const factory = createComponentTestFactory(StatCard);

describe('StatCard', () => {
  it('shows muted secondary context beside the primary value', () => {
    const view = factory.render({
      title: 'Unique Species',
      value: '42',
      secondaryValue: '6 new species',
      subtitle: 'Last 7 days',
    });

    const secondary = view.getByText('6 new species');
    expect(secondary).toHaveClass('opacity-50');
    expect(secondary.parentElement).toContainElement(view.getByText('42'));
  });

  it('hides secondary context while loading', () => {
    const view = factory.render({
      title: 'Unique Species',
      value: '42',
      secondaryValue: '6 new species',
      isLoading: true,
    });

    expect(view.queryByText('6 new species')).not.toBeInTheDocument();
    expect(view.getByText('...')).toBeInTheDocument();
  });
});
