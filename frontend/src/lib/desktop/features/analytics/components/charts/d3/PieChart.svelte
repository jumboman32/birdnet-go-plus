<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { arc, pie } from 'd3-shape';
  import type { PieArcDatum } from 'd3-shape';
  import { select } from 'd3-selection';
  import BaseChart from './BaseChart.svelte';
  import { ChartTooltip } from './utils/interactions';
  import { generateSpeciesColors, type ChartRenderContext } from './utils/theme';

  /** One pie slice; aggregate categories can opt out of species navigation. */
  export interface PieChartDatum {
    label: string;
    value: number;
    clickable?: boolean;
  }

  interface Props {
    data: PieChartDatum[];
    height?: number;
    ariaLabel: string;
    valueTooltipLabel: string;
    formatValue?: (_value: number) => string;
    formatPercentage?: (_value: number) => string;
    onSliceClick?: (_datum: PieChartDatum) => void;
  }

  let {
    data,
    height = 400,
    ariaLabel,
    valueTooltipLabel,
    formatValue = String,
    formatPercentage = value => value.toFixed(1),
    onSliceClick,
  }: Props = $props();
  const MARGIN = { top: 8, right: 8, bottom: 8, left: 8 };
  const PERCENT_SCALE = 100;
  const SLICE_BORDER_WIDTH = 2;
  const LABEL_HORIZONTAL_SPACE = 132;
  const LABEL_LINE_LENGTH = 18;
  const LABEL_VERTICAL_GAP = 25;
  const LABEL_EDGE_PADDING = 12;
  const slices = $derived(data.filter(row => Number.isFinite(row.value) && row.value > 0));
  const total = $derived(slices.reduce((sum, row) => sum + row.value, 0));
  let container: HTMLDivElement | undefined;
  let tooltip: ChartTooltip | null = null;
  let context = $state<ChartRenderContext | null>(null);

  function percentage(value: number) {
    return `${formatPercentage(total > 0 ? (value / total) * PERCENT_SCALE : 0)}%`;
  }

  function showTooltip(row: PieChartDatum, x: number, y: number) {
    tooltip?.show({
      title: row.label,
      items: [
        { label: valueTooltipLabel, value: `${formatValue(row.value)} (${percentage(row.value)})` },
      ],
      x,
      y,
    });
  }

  function drawChart(chart: ChartRenderContext) {
    const { chartGroup, innerWidth, innerHeight, theme } = chart;
    chartGroup.selectAll('*').remove();
    tooltip?.hide();
    if (!total || innerWidth <= 0 || innerHeight <= 0) return;
    const radius = Math.max(
      0,
      Math.min(innerHeight / 2 - LABEL_EDGE_PADDING, innerWidth / 2 - LABEL_HORIZONTAL_SPACE) -
        SLICE_BORDER_WIDTH
    );
    if (radius <= 0) return;
    const shape = arc<PieArcDatum<PieChartDatum>>().innerRadius(0).outerRadius(radius);
    const segments = pie<PieChartDatum>()
      .sort(null)
      .value(row => row.value)(slices);
    const colors = generateSpeciesColors(slices.length, theme);
    const plot = chartGroup
      .append('g')
      .attr('transform', `translate(${innerWidth / 2},${innerHeight / 2})`);
    const paths = plot
      .selectAll('path.pie-slice')
      .data(segments)
      .enter()
      .append('path')
      .attr('class', 'pie-slice')
      .attr('d', shape)
      .attr('fill', (_row, index) => colors.at(index) ?? theme.primary)
      .attr('stroke', theme.background)
      .attr('stroke-width', SLICE_BORDER_WIDTH)
      .attr('role', row => (onSliceClick && row.data.clickable !== false ? 'button' : 'img'))
      .attr('tabindex', row => (onSliceClick && row.data.clickable !== false ? 0 : null))
      .attr(
        'aria-label',
        row => `${row.data.label}: ${formatValue(row.data.value)} (${percentage(row.data.value)})`
      )
      .style('cursor', row =>
        onSliceClick && row.data.clickable !== false ? 'pointer' : 'default'
      );
    paths
      .on('mouseenter', function (event: MouseEvent, row) {
        select(this).attr('stroke', theme.foreground);
        showTooltip(row.data, event.clientX, event.clientY);
      })
      .on('mousemove', (event: MouseEvent) => tooltip?.move(event.clientX, event.clientY))
      .on('mouseleave', function () {
        if (document.activeElement !== this) {
          select(this).attr('stroke', theme.background);
          tooltip?.hide();
        }
      })
      .on('focus', function (_event, row) {
        const box = this.getBoundingClientRect();
        select(this).attr('stroke', theme.foreground);
        showTooltip(row.data, box.x + box.width / 2, box.y + box.height / 2);
      })
      .on('blur', function () {
        select(this).attr('stroke', theme.background);
        tooltip?.hide();
      })
      .on('click', (_event, row) => {
        if (row.data.clickable !== false) onSliceClick?.(row.data);
      })
      .on('keydown', (event: KeyboardEvent, row) => {
        if (
          onSliceClick &&
          row.data.clickable !== false &&
          (event.key === 'Enter' || event.key === ' ')
        ) {
          event.preventDefault();
          onSliceClick(row.data);
        }
      });

    type LabelLayout = {
      segment: PieArcDatum<PieChartDatum>;
      side: -1 | 1;
      startX: number;
      startY: number;
      y: number;
    };
    const labelRadius = radius + LABEL_LINE_LENGTH;
    const labels: LabelLayout[] = segments.map(segment => {
      const angle = (segment.startAngle + segment.endAngle) / 2;
      const side: -1 | 1 = Math.sin(angle) < 0 ? -1 : 1;
      return {
        segment,
        side,
        startX: Math.sin(angle) * radius,
        startY: -Math.cos(angle) * radius,
        y: -Math.cos(angle) * labelRadius,
      };
    });
    const verticalLimit = innerHeight / 2 - LABEL_EDGE_PADDING;
    for (const side of [-1, 1] as const) {
      const sideLabels = labels.filter(label => label.side === side).sort((a, b) => a.y - b.y);
      sideLabels.forEach((label, index) => {
        const minimum = index === 0 ? -verticalLimit : sideLabels[index - 1].y + LABEL_VERTICAL_GAP;
        label.y = Math.max(label.y, minimum);
      });
      const overflow = (sideLabels.at(-1)?.y ?? 0) - verticalLimit;
      if (overflow > 0) sideLabels.forEach(label => (label.y -= overflow));
    }
    const labelX = innerWidth / 2 - LABEL_EDGE_PADDING;
    plot
      .selectAll('polyline.pie-leader')
      .data(labels)
      .enter()
      .append('polyline')
      .attr('class', 'pie-leader')
      .attr('points', label => {
        const elbowX = label.side * (radius + LABEL_LINE_LENGTH / 2);
        const endX = label.side * labelX;
        return `${label.startX},${label.startY} ${elbowX},${label.y} ${endX},${label.y}`;
      })
      .attr('fill', 'none')
      .attr('stroke', theme.foreground)
      .attr('stroke-opacity', 0.45)
      .attr('stroke-width', 1);
    const labelGroups = plot
      .selectAll('text.pie-label')
      .data(labels)
      .enter()
      .append('text')
      .attr('class', 'pie-label')
      .attr('x', label => label.side * labelX)
      .attr('y', label => label.y)
      .attr('dy', '-0.15em')
      .attr('text-anchor', label => (label.side > 0 ? 'end' : 'start'))
      .attr('fill', theme.foreground)
      .attr('font-size', 12);
    labelGroups.append('tspan').text(label => label.segment.data.label);
    labelGroups
      .append('tspan')
      .attr('x', label => label.side * labelX)
      .attr('dy', '1.15em')
      .attr('fill-opacity', 0.65)
      .text(label => percentage(label.segment.data.value));
  }

  $effect(() => {
    void slices;
    void formatValue;
    void formatPercentage;
    void valueTooltipLabel;
    void onSliceClick;
    if (context) drawChart(context);
  });
  onMount(() => {
    if (container) tooltip = new ChartTooltip(container);
  });
  onDestroy(() => {
    tooltip?.destroy();
  });
</script>

<div class="pie-chart" bind:this={container}>
  <div class="min-h-0 min-w-0">
    <BaseChart {height} margin={MARGIN} {ariaLabel} containerRole="group">
      {#snippet children(chart)}{((context = chart), '')}{/snippet}
    </BaseChart>
  </div>
</div>

<style>
  .pie-chart {
    display: grid;
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr);
  }
</style>
