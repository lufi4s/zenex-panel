# Plan: Astro + shadcn premium dashboard

Status: plan only, no redesign code written yet. The Astro app is scaffolded in `apps/astro-panel` (Astro, React, Tailwind 4, shadcn init, shadcn MCP in `.mcp.json`).

## Ground rules
- The Go API and the root helper do not change. The new UI talks to the same `/api/v1`.
- The SolidJS panel (`apps/panel`) stays live until the new UI is complete. Then it is replaced.
- The new app lives in `apps/astro-panel` and uses React + shadcn components.

## What the current panel has (all of it must exist in the new UI)
9 pages, about 35 components:
- Login, Overview, Websites, Site (tabs: Overview, Files, Domains/SSL, Database, Mail, Security/Scanner, Backups, Page cache, Logs), Domains, Mail, Server, Activity, Settings.
- Features to keep: file manager (right-click, multi-select, zip download, upload), mailboxes (generate/show/change password, created-credentials card), page cache, monitoring charts, services, storage, notifications, panel updates (with "What's new" and hidden details), backups (local and remote), cPanel migration, branding, alerts, roles.

## Phase 0: Foundation
1. Install the needed shadcn components with the shadcn MCP: sidebar, card, table, tabs, dialog, sheet, dropdown-menu, context-menu, command, chart, badge, progress, skeleton, switch, sonner (toast), tooltip, breadcrumb, form, input, select, checkbox.
2. Design system: light and dark theme, one brand color, consistent spacing and radius, Geist or Inter font, soft shadows, subtle motion.
3. Data layer: API client, TanStack Query, login and session, role checks. Reuse the logic of `apps/panel/src/api/queries.ts` (including the low-polling rules: pause when the tab is hidden, long idle intervals).
4. App shell: Astro layout, collapsible shadcn Sidebar, top bar with search (Ctrl/Cmd+K), notification bell and user menu.

## Phase 1: Core pages
5. Login: clean split-screen.
6. Overview: server health cards, live CPU/RAM/disk chart, recent activity, quick actions.
7. Websites: table or grid, status badge, search and filter, "New website" wizard in a Sheet.

## Phase 2: Site workspace
8. Site page with tabs: Overview, Files, Domains/SSL, Database, Mail, Security/Scanner, Backups, Page cache, Logs.
9. File manager: data table, right-click menu, multi-select, drag-and-drop upload, zip download.

## Phase 3: Server pages
10. Mail (mailboxes, password tools, mail server card), Domains, Server (services, storage, monitoring), Activity log, Settings (branding, backups, alerts, panel updates).

## Phase 4: Polish
11. Command palette, empty states, skeleton loading, toasts, mobile layout, accessibility.
12. Tests (Vitest and Playwright), at least the coverage of the 95 current UI tests.
13. Build output goes to `apps/api/web/dist`. `install.sh` and the update flow stay the same.

## Order
Phase 0, 1, 2, 3, 4. After the first two phases there is a working, premium-looking UI. Each phase is committed and pushed on its own.

## Decisions (owner answered)
1. Look and feel: dark-first modern. Light theme is a secondary option.
2. Retiring the SolidJS panel: the new UI runs on its own URL while it is built. When all pages are done, it is merged into the main panel (served from the same domain and API, replacing `apps/panel`).

## Progress
- Phase 0 (foundation): done. shadcn components (sidebar, card, table, tabs, dialog, sheet, dropdown-menu, context-menu, command, chart, badge, progress, skeleton, switch, sonner, tooltip, breadcrumb, input, select, checkbox, label, separator, alert, avatar, button), dark-first theme in `src/styles/global.css` (indigo brand), API client `src/lib/api.ts`, TanStack Query `src/lib/queries.ts`, shell `src/components/app/AppShell.tsx` (sidebar, sign-out, auth redirect).
- Phase 1 (core): done for login (split screen) and overview (4 metric cards, server load chart). Websites, Domains, Mail, Server, Activity, Settings are placeholders (`ComingSoon`) until Phases 2-3.
- Checks: `npm run build` passes, `npx astro check` 0 errors. Login screenshot checked in headless Edge. Overview with live data not checked: it needs the panel API on 127.0.0.1:8443 (dev proxy).
- Not yet: build output into `apps/api/web/dist` (Phase 4), tests, command palette.

## Next step
Phase 2: Websites list and the Site workspace (tabs, file manager).
