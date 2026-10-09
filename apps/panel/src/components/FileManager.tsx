import { createEffect, createMemo, createSignal, For, on, Show } from "solid-js";
import {
  ChevronRight,
  File,
  FilePlus,
  Folder,
  FolderPlus,
  Home,
  Pencil,
  RefreshCw,
  Trash2,
  X,
} from "@/components/icons";
import {
  useCreateFolder,
  useDeleteFile,
  useFileContent,
  useFolder,
  useSaveFile,
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
  const entries = (): FileEntry[] => folder.data ?? [];

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
          <div class="overflow-x-auto rounded-lg border border-border">
            <table class="w-full text-sm">
              <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
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
                      <tr class="border-t border-border hover:bg-muted/40">
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

/** Browse, edit, create and delete the files of one website. */
export function FileBrowser(props: { site: Site }) {
  const [path, setPath] = createSignal("");
  const [openFile, setOpenFile] = createSignal<string | null>(null);
  const [creating, setCreating] = createSignal<"folder" | "file" | null>(null);
  const [newName, setNewName] = createSignal("");
  const [refreshKey, setRefreshKey] = createSignal(0);
  const [createError, setCreateError] = createSignal<string | null>(null);
  const createFolder = useCreateFolder(props.site.id);
  const saveNew = useSaveFile(props.site.id);

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
