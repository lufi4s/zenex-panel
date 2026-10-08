import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";

export interface ChartSeries {
  label: string;
  color: string;
  values: number[];
}

interface LineChartProps {
  title: string;
  times: number[];
  series: ChartSeries[];
  format: (value: number) => string;
  /** Fixed scale (for example 0 to 100 for percentages). Otherwise it fits the data. */
  domain?: [number, number];
  height?: number;
}

const DEFAULT_WIDTH = 640;
const PAD = { top: 10, right: 12, bottom: 22, left: 44 };

function timeLabel(ms: number, spanMs: number): string {
  const d = new Date(ms);
  if (spanMs > 2 * 24 * 3600 * 1000) {
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  }
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

/**
 * A small, dependency-free line chart. It shows a crosshair and a tooltip on
 * hover or touch, and it reads the same on light and dark themes.
 */
export function LineChart({ title, times, series, format, domain, height = 170 }: LineChartProps) {
  const [hover, setHover] = useState<number | null>(null);
  const frame = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(DEFAULT_WIDTH);
  // Draw at the real width so text stays readable on phones and desktops alike.
  useEffect(() => {
    const el = frame.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(240, Math.floor(entry.contentRect.width))),
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  const innerW = width - PAD.left - PAD.right;
  const innerH = height - PAD.top - PAD.bottom;

  const scale = useMemo(() => {
    const values = series.flatMap((s) => s.values).filter((v) => Number.isFinite(v));
    const dataMax = values.length ? Math.max(...values) : 0;
    const lo = domain ? domain[0] : 0;
    const hi = domain ? domain[1] : dataMax > 0 ? dataMax * 1.15 : 1;
    const tMin = times[0] ?? 0;
    const tMax = times[times.length - 1] ?? 0;
    return {
      lo,
      hi,
      tMin,
      tMax,
      x: (t: number) =>
        (tMax === tMin ? innerW / 2 : ((t - tMin) / (tMax - tMin)) * innerW) + PAD.left,
      y: (v: number) => PAD.top + innerH - ((v - lo) / (hi - lo || 1)) * innerH,
    };
  }, [series, times, domain, innerW, innerH]);

  if (times.length === 0) {
    return (
      <div className="flex h-[170px] items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground">
        No data yet. Samples are taken every 30 seconds.
      </div>
    );
  }

  const ticks = [0, 1, 2, 3].map((i) => scale.lo + ((scale.hi - scale.lo) * i) / 3);
  const span = scale.tMax - scale.tMin;

  const linePath = (values: number[]) =>
    values
      .map(
        (v, i) => `${i === 0 ? "M" : "L"}${scale.x(times[i]).toFixed(1)},${scale.y(v).toFixed(1)}`,
      )
      .join(" ");

  const areaPath = (values: number[]) => {
    if (values.length === 0) return "";
    const base = PAD.top + innerH;
    return `${linePath(values)} L${scale.x(times[values.length - 1]).toFixed(1)},${base} L${scale.x(times[0]).toFixed(1)},${base} Z`;
  };

  const onPointer = (clientX: number, svg: SVGSVGElement) => {
    const rect = svg.getBoundingClientRect();
    const px = ((clientX - rect.left) / rect.width) * width;
    let best = 0;
    let bestDist = Infinity;
    times.forEach((t, i) => {
      const dist = Math.abs(scale.x(t) - px);
      if (dist < bestDist) {
        bestDist = dist;
        best = i;
      }
    });
    setHover(best);
  };

  const hoverX = hover !== null ? scale.x(times[hover]) : 0;
  const tooltipLeft = hover !== null ? (hoverX / width) * 100 : 0;
  // Keep the tooltip inside the chart near either edge.
  const tooltipAlign =
    tooltipLeft < 18
      ? "translate-x-0"
      : tooltipLeft > 82
        ? "-translate-x-full"
        : "-translate-x-1/2";

  return (
    <div ref={frame} className="relative">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="block w-full touch-none select-none"
        role="img"
        aria-label={`${title}: ${series.map((s) => `${s.label} ${format(s.values.at(-1) ?? 0)}`).join(", ")}`}
        onMouseMove={(e) => onPointer(e.clientX, e.currentTarget)}
        onTouchMove={(e) => {
          const touch = e.touches[0];
          if (touch) onPointer(touch.clientX, e.currentTarget);
        }}
        onMouseLeave={() => setHover(null)}
      >
        {ticks.map((tick) => (
          <g key={tick}>
            <line
              x1={PAD.left}
              x2={width - PAD.right}
              y1={scale.y(tick)}
              y2={scale.y(tick)}
              className="stroke-border"
              strokeWidth={1}
            />
            <text
              x={PAD.left - 6}
              y={scale.y(tick) + 4}
              textAnchor="end"
              className="fill-muted-foreground text-[10px]"
            >
              {format(tick)}
            </text>
          </g>
        ))}

        {series.map((s) => (
          <g key={s.label}>
            <path d={areaPath(s.values)} fill={s.color} opacity={0.12} />
            <path
              d={linePath(s.values)}
              fill="none"
              stroke={s.color}
              strokeWidth={2}
              strokeLinejoin="round"
            />
          </g>
        ))}

        <text x={PAD.left} y={height - 6} className="fill-muted-foreground text-[10px]">
          {timeLabel(scale.tMin, span)}
        </text>
        <text
          x={width - PAD.right}
          y={height - 6}
          textAnchor="end"
          className="fill-muted-foreground text-[10px]"
        >
          {timeLabel(scale.tMax, span)}
        </text>

        {hover !== null && (
          <g>
            <line
              x1={hoverX}
              x2={hoverX}
              y1={PAD.top}
              y2={PAD.top + innerH}
              className="stroke-muted-foreground"
              strokeDasharray="3 3"
            />
            {series.map((s) => (
              <circle
                key={s.label}
                cx={hoverX}
                cy={scale.y(s.values[hover] ?? 0)}
                r={4}
                fill={s.color}
                className="stroke-card"
                strokeWidth={2}
              />
            ))}
          </g>
        )}
      </svg>

      {hover !== null && (
        <div
          className={cn(
            "pointer-events-none absolute top-0 z-10 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs shadow-md",
            tooltipAlign,
          )}
          style={{ left: `${tooltipLeft}%` }}
        >
          <div className="mb-0.5 text-muted-foreground">
            {new Date(times[hover]).toLocaleString()}
          </div>
          {series.map((s) => (
            <div key={s.label} className="flex items-center gap-1.5 tabular-nums">
              <span
                className="inline-block size-2 rounded-full"
                style={{ background: s.color }}
                aria-hidden
              />
              <span className="text-muted-foreground">{s.label}</span>
              <span className="ml-auto pl-3 font-medium">{format(s.values[hover] ?? 0)}</span>
            </div>
          ))}
        </div>
      )}
      <span className="sr-only">{title}</span>
    </div>
  );
}
