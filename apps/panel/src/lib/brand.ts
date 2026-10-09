import { createSignal } from "solid-js";
import type { Branding } from "@/api/types";

const [assetVersion, setAssetVersion] = createSignal(0);

/** Changes after a logo or favicon is uploaded or removed, so browsers fetch the new file. */
export const brandAssetVersion = assetVersion;

export function touchBrandAssets(): void {
  setAssetVersion(Date.now());
}

/** The accent colour as RGB values from a #rrggbb string. */
export function rgbOf(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** Relative luminance (WCAG), used to pick readable text on the accent colour. */
export function luminance(hex: string): number {
  const [r, g, b] = rgbOf(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** Dark text on light accents, white text on dark accents. */
export function foregroundFor(hex: string): string {
  return luminance(hex) > 0.4 ? "#0b0f14" : "#ffffff";
}

/** Uses the uploaded favicon when there is one; otherwise the default icon in index.html stays. */
function applyFavicon(b: Branding): void {
  const selector = 'link[rel="icon"][data-zenex-favicon]';
  const existing = document.head.querySelector<HTMLLinkElement>(selector);
  if (!b.has_favicon) {
    existing?.remove();
    return;
  }
  const link = existing ?? document.createElement("link");
  if (!existing) {
    link.rel = "icon";
    link.dataset.zenexFavicon = "";
    document.head.appendChild(link);
  }
  link.href = `/api/v1/branding/favicon?v=${brandAssetVersion()}`;
}

/** Applies the branding to the page title, the accent colour and the favicon. */
export function applyBranding(b: Branding): void {
  document.title = b.name;
  const root = document.documentElement.style;
  root.setProperty("--primary", b.primary_color);
  root.setProperty("--ring", b.primary_color);
  root.setProperty("--primary-foreground", foregroundFor(b.primary_color));
  applyFavicon(b);
}

export const ACCENT_PRESETS = [
  "#3b6fd4",
  "#0f766e",
  "#7c3aed",
  "#be185d",
  "#c2410c",
  "#15803d",
  "#334155",
];
