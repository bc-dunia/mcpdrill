import { useRef, useCallback, memo, ReactNode, ComponentType } from 'react';
import {
  ResponsiveContainer,
  LineChart,
  AreaChart,
  Line,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
} from 'recharts';
import { Icon, IconName } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

// Series configuration for Line charts
export interface LineSeriesConfig {
  dataKey: string;
  color: string;
  name: string;
  dashed?: boolean;
}

// Series configuration for Area charts
export interface AreaSeriesConfig {
  dataKey: string;
  color: string;
  name: string;
  gradientId: string;
  stackId?: string;
}

// Gradient definition
export interface GradientDef {
  id: string;
  color: string;
  topOpacity?: number;
  bottomOpacity?: number;
}

// Reference line configuration
export interface ReferenceLineConfig {
  y: number;
  stroke: string;
  label?: string;
  dashed?: boolean;
}

// Y-axis configuration
export interface YAxisConfig {
  formatter?: (value: number) => string;
  domain?: [number | 'auto', number | 'auto'];
}

// Base props for all chart wrappers
export interface BaseChartProps<T> {
  data: T[];
  loading?: boolean;
  title: string;
  titleTooltip?: string;
  chartId: string;
  emptyIcon: IconName;
  emptyMessage: string;
  dataSummary: string;
  height?: number;
  headerActions?: ReactNode;
  footer?: ReactNode;
}

// Props for LineChart variant
export interface LineChartWrapperProps<T> extends BaseChartProps<T> {
  chartType: 'line';
  series: LineSeriesConfig[];
  yAxisConfig?: YAxisConfig;
  customTooltip?: ComponentType<{ active?: boolean; payload?: unknown[]; label?: string }>;
  showLegend?: boolean;
}

// Props for AreaChart variant
export interface AreaChartWrapperProps<T> extends BaseChartProps<T> {
  chartType: 'area';
  series: AreaSeriesConfig[];
  gradients: GradientDef[];
  yAxisConfig?: YAxisConfig;
  customTooltip?: ComponentType<{ active?: boolean; payload?: unknown[]; label?: string }>;
  referenceLine?: ReferenceLineConfig;
  showLegend?: boolean;
}

export type ChartWrapperProps<T> = LineChartWrapperProps<T> | AreaChartWrapperProps<T>;

function BaseChartComponent<T extends { time: string }>({
  data,
  loading,
  title,
  titleTooltip,
  chartId,
  emptyIcon,
  emptyMessage,
  dataSummary,
  height = 280,
  headerActions,
  footer,
  ...chartProps
}: ChartWrapperProps<T>) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `${chartId}-${Date.now()}`);
  }, [chartId]);

  const ariaLabel = `${title} chart`;
  const titleId = `${chartId}-chart-title`;
  const summaryId = `${chartId}-chart-summary`;

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label={ariaLabel}>
        <div className="metrics-chart-header">
          <h3>{title}</h3>
        </div>
        <div className="metrics-chart-loading" role="status" aria-live="polite">
          <div className="spinner" aria-hidden="true" />
          <span>Loading metrics...</span>
        </div>
      </div>
    );
  }

  if (!data.length) {
    return (
      <div className="metrics-chart-container" role="region" aria-label={ariaLabel}>
        <div className="metrics-chart-header">
          <h3>{title}</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true">
            <Icon name={emptyIcon} size="xl" />
          </span>
          <span>{emptyMessage}</span>
        </div>
      </div>
    );
  }

  const renderChart = () => {
    const commonXAxisProps = {
      dataKey: 'time' as const,
      stroke: 'var(--text-muted)',
      fontSize: 11,
      tickLine: false,
    };

    const yAxisConfig = chartProps.yAxisConfig;
    const commonYAxisProps = {
      stroke: 'var(--text-muted)',
      fontSize: 11,
      tickLine: false,
      axisLine: false,
      tickFormatter: yAxisConfig?.formatter,
      domain: yAxisConfig?.domain,
    };

    const TooltipComponent = chartProps.customTooltip;

    if (chartProps.chartType === 'line') {
      return (
        <LineChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
          <XAxis {...commonXAxisProps} />
          <YAxis {...commonYAxisProps} />
          {TooltipComponent ? <Tooltip content={<TooltipComponent />} /> : <Tooltip />}
          {chartProps.showLegend && (
            <Legend wrapperStyle={{ fontSize: '11px', paddingTop: '8px' }} />
          )}
          {chartProps.series.map((s) => (
            <Line
              key={s.dataKey}
              type="monotone"
              dataKey={s.dataKey}
              stroke={s.color}
              strokeWidth={2}
              strokeDasharray={s.dashed ? '5 5' : undefined}
              dot={data.length <= 2 ? { r: 4, fill: s.color } : false}
              activeDot={{ r: 4, fill: s.color }}
              name={s.name}
            />
          ))}
        </LineChart>
      );
    }

    // Area chart
    return (
      <AreaChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
        <defs>
          {chartProps.gradients.map((g) => (
            <linearGradient key={g.id} id={g.id} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={g.color} stopOpacity={g.topOpacity ?? 0.4} />
              <stop offset="95%" stopColor={g.color} stopOpacity={g.bottomOpacity ?? 0.05} />
            </linearGradient>
          ))}
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
        <XAxis {...commonXAxisProps} />
        <YAxis {...commonYAxisProps} />
        {TooltipComponent ? <Tooltip content={<TooltipComponent />} /> : <Tooltip />}
        {chartProps.showLegend && (
          <Legend wrapperStyle={{ fontSize: '11px', paddingTop: '8px' }} />
        )}
        {chartProps.referenceLine && (
          <ReferenceLine
            y={chartProps.referenceLine.y}
            stroke={chartProps.referenceLine.stroke}
            strokeDasharray={chartProps.referenceLine.dashed !== false ? '5 5' : undefined}
            strokeWidth={2}
            label={
              chartProps.referenceLine.label
                ? {
                    value: chartProps.referenceLine.label,
                    position: 'right',
                    fill: chartProps.referenceLine.stroke,
                    fontSize: 11,
                  }
                : undefined
            }
          />
        )}
        {chartProps.series.map((s) => (
          <Area
            key={s.dataKey}
            type="monotone"
            dataKey={s.dataKey}
            stackId={s.stackId}
            stroke={s.color}
            strokeWidth={2}
            fill={`url(#${s.gradientId})`}
            dot={data.length <= 2 ? { r: 4, fill: s.color } : false}
            name={s.name}
          />
        ))}
      </AreaChart>
    );
  };

  return (
    <div
      className="metrics-chart-container"
      ref={chartRef}
      role="region"
      aria-labelledby={titleId}
      aria-describedby={summaryId}
    >
      <div className="metrics-chart-header">
        <h3 id={titleId} title={titleTooltip}>
          {title}
        </h3>
        <div className="chart-header-actions">
          {headerActions}
          <button
            className="btn btn-ghost btn-sm"
            onClick={exportAsPng}
            aria-label={`Export ${title.toLowerCase()} chart as PNG`}
          >
            <Icon name="download" size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>
      <p id={summaryId} className="sr-only">
        {dataSummary}
      </p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={height}>
          {renderChart()}
        </ResponsiveContainer>
      </div>
      {footer}
    </div>
  );
}

export const BaseChart = memo(BaseChartComponent) as <T extends { time: string }>(
  props: ChartWrapperProps<T>
) => JSX.Element;
