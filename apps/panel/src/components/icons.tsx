// Inline SVG icons (24×24 grid, stroked with currentColor). Replaces an icon package.
import { splitProps, type JSX } from "solid-js";

type IconProps = JSX.SvgSVGAttributes<SVGSVGElement> & { class?: string };

// A shape is a DOM node. Every render gets its own copy: one shared node would be moved from
// icon to icon, and so vanish from every place but the last one shown.
function copyShape(shape: JSX.Element): JSX.Element {
  if (Array.isArray(shape)) return shape.map(copyShape);
  if (shape instanceof Node) return shape.cloneNode(true);
  return shape;
}

function icon(name: string, body: JSX.Element) {
  return function Icon(props: IconProps) {
    const [own, rest] = splitProps(props, ["class"]);
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width={2}
        stroke-linecap="round"
        stroke-linejoin="round"
        class={own.class ?? "size-4"}
        aria-hidden="true"
        data-icon={name}
        {...rest}
      >
        {copyShape(body)}
      </svg>
    );
  };
}

export const Activity = icon("Activity", <path d="M22 12h-4l-3 9L9 3l-3 9H2" />);
export const AlertTriangle = icon(
  "AlertTriangle",
  <>
    <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
    <path d="M12 9v4M12 17h.01" />
  </>,
);
export const ArrowRight = icon("ArrowRight", <path d="M5 12h14M12 5l7 7-7 7" />);
export const ArrowUpCircle = icon(
  "ArrowUpCircle",
  <>
    <circle cx="12" cy="12" r="10" />
    <path d="M12 16V8M8 12l4-4 4 4" />
  </>,
);
export const Bell = icon(
  "Bell",
  <>
    <path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
    <path d="M13.7 21a2 2 0 0 1-3.4 0" />
  </>,
);
export const Check = icon("Check", <path d="M20 6 9 17l-5-5" />);
export const CheckCheck = icon("CheckCheck", <path d="M18 6 7 17l-5-5M22 10l-7.5 7.5L13 16" />);
export const CheckCircle2 = icon(
  "CheckCircle2",
  <>
    <circle cx="12" cy="12" r="10" />
    <path d="m9 12 2 2 4-4" />
  </>,
);
export const ChevronRight = icon("ChevronRight", <path d="m9 18 6-6-6-6" />);
export const ChevronsUpDown = icon("ChevronsUpDown", <path d="m7 15 5 5 5-5M7 9l5-5 5 5" />);
export const CircleDashed = icon(
  "CircleDashed",
  <circle cx="12" cy="12" r="10" stroke-dasharray="3 4" />,
);
export const Copy = icon(
  "Copy",
  <>
    <rect x="9" y="9" width="13" height="13" rx="2" />
    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
  </>,
);
export const Cpu = icon(
  "Cpu",
  <>
    <rect x="4" y="4" width="16" height="16" rx="2" />
    <rect x="9" y="9" width="6" height="6" />
    <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
  </>,
);
export const ExternalLink = icon(
  "ExternalLink",
  <>
    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
    <path d="M15 3h6v6M10 14 21 3" />
  </>,
);
export const Eye = icon(
  "Eye",
  <>
    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
    <circle cx="12" cy="12" r="3" />
  </>,
);
export const EyeOff = icon(
  "EyeOff",
  <>
    <path d="M17.9 17.9A10.1 10.1 0 0 1 12 20c-7 0-11-8-11-8a18.5 18.5 0 0 1 5.1-5.9M9.9 4.2A9.1 9.1 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.2 3.2M1 1l22 22" />
  </>,
);
export const File = icon(
  "File",
  <>
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
    <path d="M14 2v6h6" />
  </>,
);
export const FilePlus = icon(
  "FilePlus",
  <>
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
    <path d="M14 2v6h6M12 18v-6M9 15h6" />
  </>,
);
export const Folder = icon(
  "Folder",
  <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />,
);
export const FolderPlus = icon(
  "FolderPlus",
  <>
    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
    <path d="M12 11v6M9 14h6" />
  </>,
);
export const Globe = icon(
  "Globe",
  <>
    <circle cx="12" cy="12" r="10" />
    <path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
  </>,
);
export const HardDrive = icon(
  "HardDrive",
  <>
    <path d="M22 12H2M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1z" />
    <path d="M6 16h.01M10 16h.01" />
  </>,
);
export const Home = icon(
  "Home",
  <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM9 22V12h6v10" />,
);
export const Info = icon(
  "Info",
  <>
    <circle cx="12" cy="12" r="10" />
    <path d="M12 16v-4M12 8h.01" />
  </>,
);
export const Layers = icon(
  "Layers",
  <path d="m12 2 10 5-10 5L2 7zM2 17l10 5 10-5M2 12l10 5 10-5" />,
);
export const LayoutDashboard = icon(
  "LayoutDashboard",
  <path d="M3 3h7v9H3zM14 3h7v5h-7zM14 12h7v9h-7zM3 16h7v5H3z" />,
);
export const LogOut = icon(
  "LogOut",
  <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />,
);
export const Loader2 = icon("Loader2", <path d="M21 12a9 9 0 1 1-6.2-8.6" />);
export const Menu = icon("Menu", <path d="M3 12h18M3 6h18M3 18h18" />);
export const MemoryStick = icon(
  "MemoryStick",
  <>
    <path d="M6 19v-3M10 19v-3M14 19v-3M18 19v-3" />
    <rect x="2" y="6" width="20" height="10" rx="2" />
    <path d="M6 6V3M10 6V3M14 6V3M18 6V3" />
  </>,
);
export const Pause = icon("Pause", <path d="M6 4h4v16H6zM14 4h4v16h-4z" />);
export const Pencil = icon(
  "Pencil",
  <path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z" />,
);
export const Play = icon("Play", <path d="M6 4l14 8-14 8z" />);
export const Plus = icon("Plus", <path d="M12 5v14M5 12h14" />);
export const RefreshCw = icon(
  "RefreshCw",
  <path d="M21 12a9 9 0 0 0-9-9 9.7 9.7 0 0 0-6.7 2.7L3 8M3 3v5h5M3 12a9 9 0 0 0 9 9 9.7 9.7 0 0 0 6.7-2.7L21 16M21 21v-5h-5" />,
);
export const Search = icon(
  "Search",
  <path d="m21 21-4.3-4.3M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z" />,
);
export const Server = icon(
  "Server",
  <>
    <rect x="2" y="3" width="20" height="8" rx="2" />
    <rect x="2" y="13" width="20" height="8" rx="2" />
    <path d="M6 7h.01M6 17h.01" />
  </>,
);
export const ShieldCheck = icon(
  "ShieldCheck",
  <>
    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    <path d="m9 12 2 2 4-4" />
  </>,
);
export const Timer = icon(
  "Timer",
  <>
    <circle cx="12" cy="13" r="8" />
    <path d="M12 9v4M9 2h6" />
  </>,
);
export const Trash2 = icon(
  "Trash2",
  <>
    <path d="M3 6h18M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
    <path d="M10 11v6M14 11v6M9 6V4h6v2" />
  </>,
);
export const X = icon("X", <path d="M18 6 6 18M6 6l12 12" />);
export const XCircle = icon(
  "XCircle",
  <>
    <circle cx="12" cy="12" r="10" />
    <path d="m15 9-6 6M9 9l6 6" />
  </>,
);

/** Sliders: the settings icon (the palette icon suggests theming, not settings). */
export const Settings = icon(
  "Settings",
  <path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" />,
);
