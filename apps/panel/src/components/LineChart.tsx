import { createEffect, createMemo, createSignal, For, onCleanup, Show } from "solid-js";
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
export function LineChart(props: LineChartProps) {
  const height = () => props.height ?? 170;
  const [hover, setHover] = createSignal<number | null>(null);
  const [frame, setFrame] = createSignal<HTMLDivElement>();
  const [width, setWidth] = createSignal(DEFAULT_WIDTH);

  // Draw at the real width so text stays readable on phones and desktops alike.
  // Observed whenever the chart body appears, which can be after the data loads.
  createEffect(() => {
    const el = frame();
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(240, Math.floor(entry.contentRect.width))),
    );
    observer.observe(el);
    onCleanup(() => observer.disconnect());
  });

  const innerW = () => width() - PAD.left - PAD.right;
  const innerH = () => height() - PAD.top - PAD.bottom;

  const scale = createMemo(() => {
    const values = props.series.flatMap((s) => s.values).filter((v) => Number.isFinite(v));
    const dataMax = values.length ? Math.max(...values) : 0;
    const domain = props.domain;
    const lo = domain ? domain[0] : 0;
    const hi = domain ? domain[1] : dataMax > 0 ? dataMax * 1.15 : 1;
    const tMin = props.times[0] ?? 0;
    const tMax = props.times[props.times.length - 1] ?? 0;
    const w = innerW();
    const h = innerH();
    return {
      lo,
      hi,
      tMin,
      tMax,
      x: (t: number) => (tMax === tMin ? w / 2 : ((t - tMin) / (tMax - tMin)) * w) + PAD.left,
      y: (v: number) => PAD.top + h - ((v - lo) / (hi - lo || 1)) * h,
    };
  });

  const ticks = () => {
    const s = scale();
    return [0, 1, 2, 3].map((i) => s.lo + ((s.hi - s.lo) * i) / 3);
  };
  const span = () => scale().tMax - scale().tMin;

  const linePath = (values: number[]) =>
    values
      .map(
        (v, i) =>
          `${i === 0 ? "M" : "L"}${scale().x(props.times[i]).toFixed(1)},${scale().y(v).toFixed(1)}`,
      )
      .join(" ");

  const areaPath = (values: number[]) => {
    if (values.length === 0) return "";
    const base = PAD.top + innerH();
    return `${linePath(values)} L${scale()
      .x(props.times[values.length - 1])
      .toFixed(1)},${base} L${scale().x(props.times[0]).toFixed(1)},${base} Z`;
  };

  const onPointer = (clientX: number, svg: SVGSVGElement) => {
    const rect = svg.getBoundingClientRect();
    const px = ((clientX - rect.left) / rect.width) * width();
    let best = 0;
    let bestDist = Infinity;
    props.times.forEach((t, i) => {
      const dist = Math.abs(scale().x(t) - px);
      if (dist < bestDist) {
        bestDist = dist;
        best = i;
      }
    });
    setHover(best);
  };

  const hoverX = () => {
    const h = hover();
    return h !== null ? scale().x(props.times[h]) : 0;
  };
  const tooltipLeft = () => {
    const h = hover();
    return h !== null ? (hoverX() / width()) * 100 : 0;
  };
  // Keep the tooltip inside the chart near either edge.
  const tooltipAlign = () =>
    tooltipLeft() < 18
      ? "translate-x-0"
      : tooltipLeft() > 82
        ? "-translate-x-full"
        : "-translate-x-1/2";

  return (
    <Show
      when={props.times.length > 0}
      fallback={
        <div class="flex h-[170px] items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground">
          No data yet. Samples are taken every 30 seconds.
        </div>
      }
    >
      <div ref={setFrame} class="relative">
        <svg
          viewBox={`0 0 ${width()} ${height()}`}
          class="block w-full touch-none select-none"
          role="img"
          aria-label={`${props.title}: ${props.series.map((s) => `${s.label} ${props.format(s.values.at(-1) ?? 0)}`).join(", ")}`}
          onMouseMove={(e) => onPointer(e.clientX, e.currentTarget)}
          onTouchMove={(e) => {
            const touch = e.touches[0];
            if (touch) onPointer(touch.clientX, e.currentTarget);
          }}
          onMouseLeave={() => setHover(null)}
        >
          <For each={ticks()}>
            {(tick) => (
              <g>
                <line
                  x1={PAD.left}
                  x2={width() - PAD.right}
                  y1={scale().y(tick)}
                  y2={scale().y(tick)}
                  class="stroke-border"
                  stroke-width={1}
                />
                <text
                  x={PAD.left - 6}
                  y={scale().y(tick) + 4}
                  text-anchor="end"
                  class="fill-muted-foreground text-[10px]"
                >
                  {props.format(tick)}
                </text>
              </g>
            )}
          </For>

          <For each={props.series}>
            {(s) => (
              <g>
                <path d={areaPath(s.values)} fill={s.color} opacity={0.12} />
                <path
                  d={linePath(s.values)}
                  fill="none"
                  stroke={s.color}
                  stroke-width={2}
                  stroke-linejoin="round"
                />
              </g>
            )}
          </For>

          <text x={PAD.left} y={height() - 6} class="fill-muted-foreground text-[10px]">
            {timeLabel(scale().tMin, span())}
          </text>
          <text
            x={width() - PAD.right}
            y={height() - 6}
            text-anchor="end"
            class="fill-muted-foreground text-[10px]"
          >
            {timeLabel(scale().tMax, span())}
          </text>

          <Show when={hover() !== null}>
            <g>
              <line
                x1={hoverX()}
                x2={hoverX()}
                y1={PAD.top}
                y2={PAD.top + innerH()}
                class="stroke-muted-foreground"
                stroke-dasharray="3 3"
              />
              <For each={props.series}>
                {(s) => (
                  <circle
                    cx={hoverX()}
                    cy={scale().y(s.values[hover() ?? 0] ?? 0)}
                    r={4}
                    fill={s.color}
                    class="stroke-card"
                    stroke-width={2}
                  />
                )}
              </For>
            </g>
          </Show>
        </svg>

        <Show when={hover() !== null}>
          <div
            class={cn(
              "pointer-events-none absolute top-0 z-10 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs shadow-md",
              tooltipAlign(),
            )}
            style={{ left: `${tooltipLeft()}%` }}
          >
            <div class="mb-0.5 text-muted-foreground">
              {new Date(props.times[hover() ?? 0]).toLocaleString()}
            </div>
            <For each={props.series}>
              {(s) => (
                <div class="flex items-center gap-1.5 tabular-nums">
                  <span
                    class="inline-block size-2 rounded-full"
                    style={{ background: s.color }}
                    aria-hidden="true"
                  />
                  <span class="text-muted-foreground">{s.label}</span>
                  <span class="ml-auto pl-3 font-medium">
                    {props.format(s.values[hover() ?? 0] ?? 0)}
                  </span>
                </div>
              )}
            </For>
          </div>
        </Show>
        <span class="sr-only">{props.title}</span>
      </div>
    </Show>
  );
}
