import type { CSSProperties } from "react";

type BaseChartProps = {
  width?: number;
  height?: number;
  style?: CSSProperties;
};

export type ChartPoint = {
  label: string;
  value: number;
};

export type StackedBar = {
  label: string;
  segments: Array<{
    value: number;
    color: string;
  }>;
};

export type HeatmapCell = {
  x: number;
  y: number;
  value: number;
};

export function TimeSeriesChart({ points, width = 480, height = 180, style }: BaseChartProps & { points: ChartPoint[] }) {
  const values = points.map((point) => point.value);
  const maxValue = Math.max(1, ...values);
  const stepX = points.length > 1 ? width / (points.length - 1) : width;
  const path = points
    .map((point, index) => {
      const x = index * stepX;
      const y = height - (point.value / maxValue) * (height - 16) - 8;
      return `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`;
    })
    .join(" ");

  return (
    <svg viewBox={`0 0 ${width} ${height}`} style={{ width: "100%", height: "auto", ...style }} role="img" aria-label="time series chart">
      <path d={path} fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function StackedBars({ bars, width = 480, height = 180, style }: BaseChartProps & { bars: StackedBar[] }) {
  const barWidth = bars.length > 0 ? width / bars.length : width;
  const totals = bars.map((bar) => bar.segments.reduce((sum, segment) => sum + segment.value, 0));
  const maxValue = Math.max(1, ...totals);

  return (
    <svg viewBox={`0 0 ${width} ${height}`} style={{ width: "100%", height: "auto", ...style }} role="img" aria-label="stacked bar chart">
      {bars.map((bar, index) => {
        let offset = 0;
        const x = index * barWidth + barWidth * 0.18;
        const innerWidth = barWidth * 0.64;
        return bar.segments.map((segment, segmentIndex) => {
          const segmentHeight = (segment.value / maxValue) * (height - 20);
          const y = height - offset - segmentHeight - 6;
          offset += segmentHeight;
          return <rect key={`${bar.label}-${segmentIndex}`} x={x} y={y} width={innerWidth} height={segmentHeight} rx="6" fill={segment.color} />;
        });
      })}
    </svg>
  );
}

export function DonutOrCoverageRing({
  value,
  total,
  size = 132,
  thickness = 14,
  trackColor = "rgba(127, 140, 141, 0.18)",
  fillColor = "#0f766e",
  style,
}: BaseChartProps & {
  value: number;
  total: number;
  size?: number;
  thickness?: number;
  trackColor?: string;
  fillColor?: string;
}) {
  const normalizedTotal = Math.max(total, 1);
  const ratio = Math.max(0, Math.min(1, value / normalizedTotal));
  const radius = size / 2 - thickness;
  const circumference = 2 * Math.PI * radius;
  const dashOffset = circumference * (1 - ratio);

  return (
    <svg viewBox={`0 0 ${size} ${size}`} style={{ width: size, height: size, ...style }} role="img" aria-label="coverage ring">
      <circle cx={size / 2} cy={size / 2} r={radius} fill="none" stroke={trackColor} strokeWidth={thickness} />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        stroke={fillColor}
        strokeWidth={thickness}
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={dashOffset}
        transform={`rotate(-90 ${size / 2} ${size / 2})`}
      />
      <text x="50%" y="50%" textAnchor="middle" dominantBaseline="central" style={{ fontSize: 18, fontWeight: 700, fill: "currentColor" }}>
        {Math.round(ratio * 100)}%
      </text>
    </svg>
  );
}

export function HeatmapGrid({
  cells,
  columns,
  rows,
  width = 480,
  height = 180,
  style,
}: BaseChartProps & {
  cells: HeatmapCell[];
  columns: number;
  rows: number;
}) {
  const cellWidth = width / Math.max(columns, 1);
  const cellHeight = height / Math.max(rows, 1);
  const maxValue = Math.max(1, ...cells.map((cell) => cell.value));

  return (
    <svg viewBox={`0 0 ${width} ${height}`} style={{ width: "100%", height: "auto", ...style }} role="img" aria-label="activity heatmap">
      {cells.map((cell) => {
        const intensity = cell.value / maxValue;
        const fill = `rgba(15, 118, 110, ${0.12 + intensity * 0.78})`;
        return (
          <rect
            key={`${cell.x}-${cell.y}`}
            x={cell.x * cellWidth + 1}
            y={cell.y * cellHeight + 1}
            width={Math.max(cellWidth - 2, 0)}
            height={Math.max(cellHeight - 2, 0)}
            rx="4"
            fill={fill}
          />
        );
      })}
    </svg>
  );
}

export function HealthSparkline({ values, width = 180, height = 48, style }: BaseChartProps & { values: number[] }) {
  const maxValue = Math.max(1, ...values);
  const stepX = values.length > 1 ? width / (values.length - 1) : width;
  const path = values
    .map((value, index) => {
      const x = index * stepX;
      const y = height - (value / maxValue) * (height - 10) - 5;
      return `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`;
    })
    .join(" ");

  return (
    <svg viewBox={`0 0 ${width} ${height}`} style={{ width: "100%", height: "auto", ...style }} role="img" aria-label="health sparkline">
      <path d={path} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
