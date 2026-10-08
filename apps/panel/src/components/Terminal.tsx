import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Pause, Play, Search } from "@/components/icons";
import { cn } from "@/lib/utils";

export type TerminalLevel = "info" | "warn" | "error" | "debug" | "success";

export interface TerminalLine {
  id: string | number;
  time?: string;
  level?: TerminalLevel;
  text: string;
}

const LEVEL_CLASS: Record<TerminalLevel, string> = {
  debug: "text-slate-500",
  info: "text-slate-300",
  success: "text-emerald-400",
  warn: "text-amber-300",
  error: "text-rose-400",
};

const LEVEL_LABEL: Record<TerminalLevel, string> = {
  debug: "DBG",
  info: "INF",
  success: "OK ",
  warn: "WRN",
  error: "ERR",
};

/** Keeps the lines that contain the search text, ignoring case. Pure, so it is tested. */
export function filterTerminalLines(lines: TerminalLine[], query: string): TerminalLine[] {
  const q = query.trim().toLowerCase();
  if (q === "") return lines;
  return lines.filter((l) =>
    `${l.time ?? ""} ${l.level ?? ""} ${l.text}`.toLowerCase().includes(q),
  );
}

interface TerminalProps {
  lines: TerminalLine[];
  title: string;
  emptyText?: string;
  /** Follow the newest line automatically. The user can turn this off to read older lines. */
  defaultFollow?: boolean;
  className?: string;
  maxHeight?: string;
}

/**
 * A console-style log viewer: monospace text, coloured levels, line numbers,
 * search, follow mode and copy. Its colours are fixed, so it looks the same in
 * light and dark themes, like a real terminal.
 */
export function Terminal({
  lines,
  title,
  emptyText = "No output yet.",
  defaultFollow = true,
  className,
  maxHeight = "20rem",
}: TerminalProps) {
  const [query, setQuery] = useState("");
  const [follow, setFollow] = useState(defaultFollow);
  const [copied, setCopied] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  const visible = useMemo(() => filterTerminalLines(lines, query), [lines, query]);

  useEffect(() => {
    if (follow && body.current) body.current.scrollTop = body.current.scrollHeight;
  }, [visible, follow]);

  const copy = async () => {
    const text = visible
      .map((l) => [l.time, l.level && LEVEL_LABEL[l.level], l.text].filter(Boolean).join(" "))
      .join("\n");
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div
      className={cn(
        "overflow-hidden rounded-lg border border-slate-800 bg-[#0b0f14] font-mono text-xs text-slate-300 shadow-inner",
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-2 border-b border-slate-800 bg-[#11161d] px-3 py-2">
        <span className="flex gap-1.5" aria-hidden>
          <span className="size-2.5 rounded-full bg-rose-500/80" />
          <span className="size-2.5 rounded-full bg-amber-400/80" />
          <span className="size-2.5 rounded-full bg-emerald-500/80" />
        </span>
        <span className="truncate text-slate-400">{title}</span>
        <span className="ml-auto flex items-center gap-1.5">
          <label className="relative hidden sm:block">
            <Search
              className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-slate-500"
              aria-hidden
            />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="filter"
              aria-label="Filter log lines"
              className="w-40 rounded border border-slate-700 bg-[#0b0f14] py-1 pl-6 pr-2 text-[11px] text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-slate-500"
            />
          </label>
          <button
            type="button"
            onClick={() => setFollow((v) => !v)}
            aria-pressed={follow}
            className="flex items-center gap-1 rounded px-2 py-1 text-[11px] text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            {follow ? (
              <Pause className="size-3" aria-hidden />
            ) : (
              <Play className="size-3" aria-hidden />
            )}
            {follow ? "Follow" : "Paused"}
          </button>
          <button
            type="button"
            onClick={copy}
            className="flex items-center gap-1 rounded px-2 py-1 text-[11px] text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          >
            {copied ? (
              <Check className="size-3" aria-hidden />
            ) : (
              <Copy className="size-3" aria-hidden />
            )}
            {copied ? "Copied" : "Copy"}
          </button>
        </span>
      </div>

      <div
        ref={body}
        role="log"
        aria-live="polite"
        aria-label={title}
        onScroll={(e) => {
          // Scrolling away from the bottom pauses follow mode; reaching it resumes.
          const el = e.currentTarget;
          const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 8;
          if (!atBottom && follow) setFollow(false);
        }}
        className="overflow-auto p-3 leading-5"
        style={{ maxHeight }}
      >
        {visible.length === 0 ? (
          <p className="text-slate-600">{query ? "No lines match this filter." : emptyText}</p>
        ) : (
          visible.map((line, index) => (
            <div
              key={line.id}
              className="flex gap-3 whitespace-pre-wrap break-all hover:bg-white/[0.03]"
            >
              <span className="w-8 shrink-0 select-none text-right text-slate-600 tabular-nums">
                {index + 1}
              </span>
              {line.time && (
                <span className="shrink-0 text-slate-500 tabular-nums">{line.time}</span>
              )}
              {line.level && (
                <span className={cn("shrink-0 font-semibold", LEVEL_CLASS[line.level])}>
                  {LEVEL_LABEL[line.level]}
                </span>
              )}
              <span
                className={cn("min-w-0", line.level ? LEVEL_CLASS[line.level] : "text-slate-300")}
              >
                {line.text}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

/** Finds a level word such as ERROR or WARN in a log line. Unknown lines stay plain. */
export function levelFromText(text: string): TerminalLevel | undefined {
  if (/\b(ERROR|FATAL|CRIT|error)\b/.test(text)) return "error";
  if (/\b(WARN|WARNING|warn)\b/.test(text)) return "warn";
  if (/\bDEBUG\b/.test(text)) return "debug";
  return undefined;
}
