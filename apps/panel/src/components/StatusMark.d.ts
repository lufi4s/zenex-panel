import type { CSSProperties } from "react";

export type StatusMarkStatus = "pending" | "running" | "done" | "failed" | "cancelled";

export interface StatusMarkProps {
  status?: StatusMarkStatus;
  /** 0–1; shows a determinate ring while running. */
  progress?: number;
  label?: string;
  color?: string;
  doneColor?: string;
  errorColor?: string;
  size?: number;
  strokeWidth?: number;
  dashes?: number;
  fontSize?: number;
  spinDuration?: number;
  arcLength?: number;
  drawDuration?: number;
  fillOpacity?: number;
  strike?: boolean;
  strikeDelay?: number;
  className?: string;
  style?: CSSProperties;
}

export default function StatusMark(props: StatusMarkProps): JSX.Element;
