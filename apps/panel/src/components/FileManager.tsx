import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from "solid-js";
import {
  ChevronRight,
  Download,
  File,
  FilePlus,
  Folder,
  FolderPlus,
  Home,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
  X,
} from "@/components/icons";
import {
  useCreateFolder,
  useDeleteFile,
  useFileContent,
  useFolder,
  useSaveFile,
  useUploadFile,
  downloadUrl,
} from "@/api/queries";
import { describeError } from "@/api/client";
import type { FileEntry, Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { formatBytes } from "@/lib/format";

function joinPath(folder: string, name: string): string {
  return folder ? `${folder}/${name}` : name;
}

/** Starts the browser download of the given paths of a website. */
function startDownload(siteId: string, paths: string[]) {
  const link = document.createElement("a");
  link.href = downloadUrl(siteId, paths);
  link.rel = "noopener";
  link.style.display = "none";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

interface MenuState {
  x: number;
  y: number;
  /** The full paths the menu acts on: the clicked row, or every selected row if it is one of them. */
  paths: string[];
  entry: FileEntry | null;
}

function Crumbs(props: { path: string; onGo: (p: string) => void }) {
  const parts = () => (props.path ? props.path.split("/") : []);
  return (
    <nav aria-label="Folder path" class="flex min-w-0 flex-wrap items-center gap-1 text-sm">
      <button
        type="button"
        onClick={() => props.onGo("")}
        class="flex items-center gap-1 rounded px-1.5 py-0.5 hover:bg-muted"
      >
        <Home class="size-3.5" aria-hidden="true" /> site
      </button>
      <For each={parts()}>
        {(part, i) => {
          const target = () =>
            parts()
              .slice(0, i() + 1)
              .join("/");
          return (
            <span class="flex items-center gap-1">
              <ChevronRight class="size-3.5 text-muted-foreground" aria-hidden="true" />
              <button
                type="button"
                onClick={() => props.onGo(target())}
                class="rounded px-1.5 py-0.5 hover:bg-muted"
              >
                {part}
              </button>
            </span>
          );
        }}
      </For>
    </nav>
  );
}

function Listing(props: {
  siteId: string;
  path: string;
  onOpenFolder: (p: string) => void;
  onOpenFile: (p: string) => void;
}) {
  const folder = useFolder(props.siteId, props.path, true);
  const remove = useDeleteFile(props.siteId);
  const [confirming, setConfirming] = createSignal<string | null>(null);
  const [selected, setSelected] = createSignal<string[]>([]);
  const [menu, setMenu] = createSignal<MenuState | null>(null);
  const entries = (): FileEntry[] => folder.data ?? [];
  const allFull = () => entries().map((e) => joinPath(props.path, e.name));
  const allSelected = () => entries().length > 0 && selected().length === entries().length;

  const toggle = (full: string) =>
    setSelected((cur) => (cur.includes(full) ? cur.filter((p) => p !== full) : [...cur, full]));

  const openMenu = (e: MouseEvent, entry: FileEntry) => {
    e.preventDefault();
    const full = joinPath(props.path, entry.name);
    const paths = selected().includes(full) ? selected() : [full];
    setMenu({
      x: Math.min(e.clientX, window.innerWidth - 220),
      y: Math.min(e.clientY, window.innerHeight - 180),
      paths,
      entry: paths.length === 1 ? entry : null,
    });
  };

  // The menu closes on any click elsewhere, on Escape and when the page scrolls.
  createEffect(() => {
    if (!menu()) return;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("click", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("keydown", onKey);
    onCleanup(() => {
      window.removeEventListener("click", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", onKey);
    });
  });

  return (
    <Show
      when={!folder.isPending}
      fallback={<p class="p-4 text-sm text-muted-foreground">Loading folder…</p>}
    >
      <Show
        when={!folder.isError}
        fallback={
          <Alert variant="destructive">
            <AlertDescription>{describeError(folder.error)}</AlertDescription>
          </Alert>
        }
      >
        <Show
          when={entries().length > 0}
          fallback={
            <p class="p-8 text-center text-sm text-muted-foreground">This folder is empty.</p>
          }
        >
          <Show when={selected().length > 0}>
            <div class="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2 text-sm">
              <span class="font-medium">{selected().length} selected</span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => startDownload(props.siteId, selected())}
              >
                <Download aria-hidden="true" /> Download selected
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setSelected([])}>
                Clear selection
              </Button>
            </div>
          </Show>
          <div class="overflow-x-auto rounded-lg border border-border">
            <table class="w-full text-sm">
              <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th class="w-10 px-4 py-2.5">
                    <input
                      type="checkbox"
                      aria-label="Select all"
                      checked={allSelected()}
                      onChange={() => setSelected(allSelected() ? [] : allFull())}
                    />
                  </th>
                  <th class="px-4 py-2.5 font-medium">Name</th>
                  <th class="hidden px-4 py-2.5 font-medium sm:table-cell">Size</th>
                  <th class="hidden px-4 py-2.5 font-medium md:table-cell">Modified</th>
                  <th class="px-4 py-2.5 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                <For each={entries()}>
                  {(entry) => {
                    const full = () => joinPath(props.path, entry.name);
                    const isDir = entry.type === "dir";
                    return (
                      <tr
                        class={`border-t border-border hover:bg-muted/40 ${selected().includes(full()) ? "bg-primary/5" : ""}`}
                        onContextMenu={(e) => openMenu(e, entry)}
                      >
                        <td class="w-10 px-4 py-2.5">
                          <input
                            type="checkbox"
                            aria-label={`Select ${entry.name}`}
                            checked={selected().includes(full())}
                            onChange={() => toggle(full())}
                          />
                        </td>
                        <td class="px-4 py-2.5">
                          <button
                            type="button"
                            onClick={() =>
                              isDir
                                ? props.onOpenFolder(full())
                                : entry.editable && props.onOpenFile(full())
                            }
                            disabled={!isDir && !entry.editable}
                            class="flex items-center gap-2 text-left disabled:cursor-default disabled:text-muted-foreground"
                            title={
                              !isDir && !entry.editable
                                ? "This file type cannot be edited here"
                                : undefined
                            }
                          >
                            {isDir ? (
                              <Folder class="size-4 text-primary" aria-hidden="true" />
                            ) : (
                              <File class="size-4 text-muted-foreground" aria-hidden="true" />
                            )}
                            <span class="font-medium">{entry.name}</span>
                          </button>
                        </td>
                        <td class="hidden px-4 py-2.5 tabular-nums text-muted-foreground sm:table-cell">
                          {isDir ? "—" : formatBytes(entry.size)}
                        </td>
                        <td class="hidden px-4 py-2.5 text-muted-foreground md:table-cell">
                          {new Date(entry.modified).toLocaleString(undefined, {
                            dateStyle: "medium",
                            timeStyle: "short",
                          })}
                        </td>
                        <td class="px-4 py-2.5 text-right">
                          <div class="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => startDownload(props.siteId, [full()])}
                              aria-label={`Download ${entry.name}`}
                              title={isDir ? "Download as a zip archive" : "Download"}
                            >
                              <Download aria-hidden="true" />
                            </Button>
                            <Show when={!isDir && entry.editable}>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => props.onOpenFile(full())}
                                aria-label={`Edit ${entry.name}`}
                              >
                                <Pencil aria-hidden="true" />
                              </Button>
                            </Show>
                            <Show
                              when={confirming() === full()}
                              fallback={
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => setConfirming(full())}
                                  aria-label={`Delete ${entry.name}`}
                                >
                                  <Trash2 aria-hidden="true" />
                                </Button>
                              }
                            >
                              <Button
                                variant="destructive"
                                size="sm"
                                disabled={remove.isPending}
                                onClick={() =>
                                  remove.mutate(full(), { onSettled: () => setConfirming(null) })
                                }
                              >
                                Confirm delete
                              </Button>
                            </Show>
                          </div>
                        </td>
                      </tr>
                    );
                  }}
                </For>
              </tbody>
            </table>
            <Show when={remove.isError}>
              <div class="p-3">
                <Alert variant="destructive">
                  <AlertDescription>{describeError(remove.error)}</AlertDescription>
                </Alert>
              </div>
            </Show>
          </div>
          <Show when={menu()}>
            {(m) => (
              <div
                role="menu"
                aria-label="File actions"
                class="fixed z-50 min-w-48 rounded-md border border-border bg-popover p-1 text-sm shadow-md"
                style={{ left: `${m().x}px`, top: `${m().y}px` }}
                onClick={(e) => e.stopPropagation()}
                onContextMenu={(e) => e.preventDefault()}
              >
                <Show when={m().entry}>
                  {(entry) => (
                    <Show when={entry().type === "dir" || entry().editable}>
                      <button
                        type="button"
                        role="menuitem"
                        class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-muted"
                        onClick={() => {
                          const target = m().paths[0];
                          setMenu(null);
                          if (entry().type === "dir") props.onOpenFolder(target);
                          else props.onOpenFile(target);
                        }}
                      >
                        {entry().type === "dir" ? "Open" : "Edit"}
                      </button>
                    </Show>
                  )}
                </Show>
                <button
                  type="button"
                  role="menuitem"
                  class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-muted"
                  onClick={() => {
                    const paths = m().paths;
                    setMenu(null);
                    startDownload(props.siteId, paths);
                  }}
                >
                  <Download class="size-4" aria-hidden="true" />
                  {m().paths.length > 1
                    ? `Download ${m().paths.length} items as zip`
                    : m().entry?.type === "dir"
                      ? "Download folder as zip"
                      : "Download"}
                </button>
              </div>
            )}
          </Show>
        </Show>
      </Show>
    </Show>
  );
}

function Editor(props: { siteId: string; path: string; onClose: () => void }) {
  const file = useFileContent(props.siteId, props.path);
  const save = useSaveFile(props.siteId);
  const [draft, setDraft] = createSignal<string | null>(null);
  const original = () => file.data?.content ?? "";
  const text = () => draft() ?? original();
  const dirty = () => draft() !== null && draft() !== original();
  const name = () => props.path.split("/").pop() ?? props.path;

  createEffect(
    on(
      () => props.path,
      () => setDraft(null),
      { defer: true },
    ),
  );

  const saveNow = () => {
    if (!dirty()) return;
    save.mutate({ path: props.path, content: text() }, { onSuccess: () => setDraft(null) });
  };

  const onKey = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      saveNow();
    }
  };

  const close = () => {
    if (dirty() && !window.confirm("You have unsaved changes. Close without saving?")) return;
    props.onClose();
  };

  const lines = () => text().split("\n").length;

  return (
    <Show
      when={!file.isPending}
      fallback={<p class="p-4 text-sm text-muted-foreground">Opening {name()}…</p>}
    >
      <Show
        when={!file.isError}
        fallback={
          <div class="space-y-3">
            <Alert variant="destructive">
              <AlertDescription>{describeError(file.error)}</AlertDescription>
            </Alert>
            <Button variant="outline" size="sm" onClick={() => props.onClose()}>
              Back to folder
            </Button>
          </div>
        }
      >
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <Pencil class="size-4 text-muted-foreground" aria-hidden="true" />
            <span class="font-medium">{name()}</span>
            <Show when={dirty()}>
              <span class="rounded bg-warning/15 px-1.5 py-0.5 text-xs text-warning">unsaved</span>
            </Show>
            <span class="ml-auto text-xs text-muted-foreground">
              {lines()} lines · Ctrl+S to save
            </span>
            <Button variant="outline" size="sm" onClick={close}>
              <X aria-hidden="true" /> Close
            </Button>
            <Button size="sm" disabled={!dirty() || save.isPending} onClick={saveNow}>
              {save.isPending ? "Saving…" : "Save"}
            </Button>
          </div>
          <textarea
            value={text()}
            onInput={(e) => setDraft(e.currentTarget.value)}
            onKeyDown={onKey}
            spellcheck={false}
            aria-label={`Edit ${name()}`}
            class="min-h-[55vh] w-full resize-y rounded-lg border border-input bg-background p-3 font-mono text-xs leading-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30"
          />
          <Show when={save.isError}>
            <Alert variant="destructive">
              <AlertDescription>{describeError(save.error)}</AlertDescription>
            </Alert>
          </Show>
        </div>
      </Show>
    </Show>
  );
}

/** Largest file the panel accepts; the server enforces the same limit. */
const MAX_UPLOAD_BYTES = 64 * 1024 * 1024;

/** Browse, edit, upload, create and delete the files of one website. */
export function FileBrowser(props: { site: Site }) {
  const [path, setPath] = createSignal("");
  const [openFile, setOpenFile] = createSignal<string | null>(null);
  const [creating, setCreating] = createSignal<"folder" | "file" | null>(null);
  const [newName, setNewName] = createSignal("");
  const [refreshKey, setRefreshKey] = createSignal(0);
  const [createError, setCreateError] = createSignal<string | null>(null);
  const createFolder = useCreateFolder(props.site.id);
  const saveNew = useSaveFile(props.site.id);
  const upload = useUploadFile(props.site.id);
  const [uploadStatus, setUploadStatus] = createSignal<string | null>(null);
  const [uploadError, setUploadError] = createSignal<string | null>(null);
  let fileInput: HTMLInputElement | undefined;

  // Files go one after another, so a failure stops the rest and the message names the file.
  const uploadPicked = async (input: HTMLInputElement) => {
    const files = Array.from(input.files ?? []);
    input.value = "";
    if (files.length === 0) return;
    setUploadError(null);
    let done = 0;
    for (const file of files) {
      if (file.size > MAX_UPLOAD_BYTES) {
        setUploadError(`${file.name} is larger than 64 MB and was not uploaded.`);
        break;
      }
      setUploadStatus(`Uploading ${file.name} (${done + 1} of ${files.length})…`);
      try {
        await upload.mutateAsync({ folder: path(), file });
        done += 1;
      } catch (err) {
        setUploadError(`${file.name}: ${describeError(err)}`);
        break;
      }
    }
    setUploadStatus(null);
    if (done > 0) setRefreshKey((k) => k + 1);
  };

  // The listing query reads its folder when it starts, so a new folder or refresh mounts a new listing.
  const listing = createMemo(() => ({ path: path(), refresh: refreshKey() }));

  const submitNew = () => {
    const name = newName().trim();
    if (!name || name.includes("/") || name === "." || name === "..") {
      setCreateError("Use a plain name without slashes.");
      return;
    }
    setCreateError(null);
    const full = joinPath(path(), name);
    if (creating() === "folder") {
      createFolder.mutate(full, {
        onSuccess: () => {
          setCreating(null);
          setNewName("");
        },
        onError: (err) => setCreateError(describeError(err)),
      });
    } else {
      saveNew.mutate(
        { path: full, content: "" },
        {
          onSuccess: () => {
            setCreating(null);
            setNewName("");
            setOpenFile(full);
          },
          onError: (err) => setCreateError(describeError(err)),
        },
      );
    }
  };

  const go = (p: string) => {
    setOpenFile(null);
    setPath(p);
    setCreating(null);
    setCreateError(null);
  };

  return (
    <Show
      when={openFile()}
      keyed
      fallback={
        <div class="space-y-4">
          <div class="flex flex-wrap items-center gap-2">
            <Crumbs path={path()} onGo={go} />
            <div class="ml-auto flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={upload.isPending}
                onClick={() => fileInput?.click()}
              >
                <Upload aria-hidden="true" /> {upload.isPending ? "Uploading…" : "Upload"}
              </Button>
              <input
                ref={(el) => {
                  fileInput = el;
                }}
                type="file"
                multiple
                class="hidden"
                aria-label="Choose files to upload"
                onChange={(e) => void uploadPicked(e.currentTarget)}
              />
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setCreating("folder");
                  setNewName("");
                  setCreateError(null);
                }}
              >
                <FolderPlus aria-hidden="true" /> New folder
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setCreating("file");
                  setNewName("");
                  setCreateError(null);
                }}
              >
                <FilePlus aria-hidden="true" /> New file
              </Button>
              <Button
                variant="ghost"
                size="sm"
                aria-label="Refresh"
                onClick={() => setRefreshKey((k) => k + 1)}
              >
                <RefreshCw aria-hidden="true" />
              </Button>
            </div>
          </div>

          <Show when={creating()}>
            {(kind) => (
              <form
                class="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/30 p-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  submitNew();
                }}
              >
                <Input
                  autofocus
                  class="w-full sm:max-w-xs"
                  placeholder={kind() === "folder" ? "folder name" : "file name, e.g. notes.txt"}
                  value={newName()}
                  onInput={(e) => setNewName(e.currentTarget.value)}
                  aria-label={kind() === "folder" ? "New folder name" : "New file name"}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={!newName().trim() || createFolder.isPending || saveNew.isPending}
                >
                  Create
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={() => setCreating(null)}>
                  Cancel
                </Button>
                <Show when={createError()}>
                  {(message) => (
                    <span class="text-xs text-destructive" role="alert">
                      {message()}
                    </span>
                  )}
                </Show>
              </form>
            )}
          </Show>

          <Show when={uploadStatus()}>
            {(message) => (
              <p class="text-xs text-muted-foreground" role="status">
                {message()}
              </p>
            )}
          </Show>
          <Show when={uploadError()}>
            {(message) => (
              <Alert variant="destructive">
                <AlertDescription>{message()}</AlertDescription>
              </Alert>
            )}
          </Show>

          <Show when={listing()} keyed>
            {(l) => (
              <Listing
                siteId={props.site.id}
                path={l.path}
                onOpenFolder={go}
                onOpenFile={(p) => setOpenFile(p)}
              />
            )}
          </Show>
        </div>
      }
    >
      {(file) => <Editor siteId={props.site.id} path={file} onClose={() => setOpenFile(null)} />}
    </Show>
  );
}
