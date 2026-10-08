import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
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
} from "lucide-react";
import {
  useCreateFolder,
  useDeleteFile,
  useFileContent,
  useFolder,
  useSaveFile,
} from "@/api/queries";
import { describeError } from "@/api/client";
import type { FileEntry, Site } from "@/api/types";
import { Alert } from "@/components/ui/badge-alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { formatBytes } from "@/lib/format";

function joinPath(folder: string, name: string): string {
  return folder ? `${folder}/${name}` : name;
}

function Crumbs({ path, onGo }: { path: string; onGo: (p: string) => void }) {
  const parts = path ? path.split("/") : [];
  return (
    <nav aria-label="Folder path" className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
      <button
        type="button"
        onClick={() => onGo("")}
        className="flex items-center gap-1 rounded px-1.5 py-0.5 hover:bg-muted"
      >
        <Home className="size-3.5" aria-hidden /> site
      </button>
      {parts.map((part, i) => {
        const target = parts.slice(0, i + 1).join("/");
        return (
          <span key={target} className="flex items-center gap-1">
            <ChevronRight className="size-3.5 text-muted-foreground" aria-hidden />
            <button
              type="button"
              onClick={() => onGo(target)}
              className="rounded px-1.5 py-0.5 hover:bg-muted"
            >
              {part}
            </button>
          </span>
        );
      })}
    </nav>
  );
}

function Listing({
  siteId,
  path,
  onOpenFolder,
  onOpenFile,
}: {
  siteId: string;
  path: string;
  onOpenFolder: (p: string) => void;
  onOpenFile: (p: string) => void;
}) {
  const folder = useFolder(siteId, path, true);
  const remove = useDeleteFile(siteId);
  const [confirming, setConfirming] = useState<string | null>(null);

  if (folder.isPending) return <p className="p-4 text-sm text-muted-foreground">Loading folder…</p>;
  if (folder.isError) return <Alert tone="danger">{describeError(folder.error)}</Alert>;
  const entries: FileEntry[] = folder.data ?? [];
  if (entries.length === 0)
    return <p className="p-6 text-center text-sm text-muted-foreground">This folder is empty.</p>;

  return (
    <div className="overflow-hidden rounded-md border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">Name</th>
            <th className="hidden px-3 py-2 font-medium sm:table-cell">Size</th>
            <th className="hidden px-3 py-2 font-medium md:table-cell">Modified</th>
            <th className="px-3 py-2 text-right font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => {
            const full = joinPath(path, entry.name);
            const isDir = entry.type === "dir";
            return (
              <tr key={entry.name} className="border-t border-border hover:bg-muted/40">
                <td className="px-3 py-2">
                  <button
                    type="button"
                    onClick={() =>
                      isDir ? onOpenFolder(full) : entry.editable && onOpenFile(full)
                    }
                    disabled={!isDir && !entry.editable}
                    className="flex items-center gap-2 text-left disabled:cursor-default disabled:text-muted-foreground"
                    title={
                      !isDir && !entry.editable ? "This file type cannot be edited here" : undefined
                    }
                  >
                    {isDir ? (
                      <Folder className="size-4 text-primary" aria-hidden />
                    ) : (
                      <File className="size-4 text-muted-foreground" aria-hidden />
                    )}
                    <span className="font-medium">{entry.name}</span>
                  </button>
                </td>
                <td className="hidden px-3 py-2 tabular-nums text-muted-foreground sm:table-cell">
                  {isDir ? "—" : formatBytes(entry.size)}
                </td>
                <td className="hidden px-3 py-2 text-muted-foreground md:table-cell">
                  {new Date(entry.modified).toLocaleString(undefined, {
                    dateStyle: "medium",
                    timeStyle: "short",
                  })}
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex justify-end gap-1">
                    {!isDir && entry.editable && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => onOpenFile(full)}
                        aria-label={`Edit ${entry.name}`}
                      >
                        <Pencil aria-hidden />
                      </Button>
                    )}
                    {confirming === full ? (
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={remove.isPending}
                        onClick={() =>
                          remove.mutate(full, { onSettled: () => setConfirming(null) })
                        }
                      >
                        Delete{isDir ? " folder" : ""}?
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setConfirming(full)}
                        aria-label={`Delete ${entry.name}`}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {remove.isError && (
        <div className="p-2">
          <Alert tone="danger">{describeError(remove.error)}</Alert>
        </div>
      )}
    </div>
  );
}

function Editor({ siteId, path, onClose }: { siteId: string; path: string; onClose: () => void }) {
  const file = useFileContent(siteId, path);
  const save = useSaveFile(siteId);
  const [draft, setDraft] = useState<string | null>(null);
  const original = file.data?.content ?? "";
  const text = draft ?? original;
  const dirty = draft !== null && draft !== original;
  const name = path.split("/").pop() ?? path;

  useEffect(() => {
    setDraft(null);
  }, [path]);

  const saveNow = () => {
    if (!dirty) return;
    save.mutate({ path, content: text }, { onSuccess: () => setDraft(null) });
  };

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      saveNow();
    }
  };

  const close = () => {
    if (dirty && !window.confirm("You have unsaved changes. Close without saving?")) return;
    onClose();
  };

  const lines = useMemo(() => text.split("\n").length, [text]);

  if (file.isPending) return <p className="p-4 text-sm text-muted-foreground">Opening {name}…</p>;
  if (file.isError) {
    return (
      <div className="space-y-3">
        <Alert tone="danger">{describeError(file.error)}</Alert>
        <Button variant="outline" size="sm" onClick={onClose}>
          Back to folder
        </Button>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Pencil className="size-4 text-muted-foreground" aria-hidden />
        <span className="font-medium">{name}</span>
        {dirty && (
          <span className="rounded bg-warning/15 px-1.5 py-0.5 text-xs text-warning">unsaved</span>
        )}
        <span className="ml-auto text-xs text-muted-foreground">
          {lines} lines · Ctrl+S to save
        </span>
        <Button variant="outline" size="sm" onClick={close}>
          <X aria-hidden /> Close
        </Button>
        <Button size="sm" disabled={!dirty || save.isPending} onClick={saveNow}>
          {save.isPending ? "Saving…" : "Save"}
        </Button>
      </div>
      <textarea
        value={text}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKey}
        spellCheck={false}
        aria-label={`Edit ${name}`}
        className="min-h-[50vh] w-full resize-y rounded-md border border-border bg-background p-3 font-mono text-xs leading-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30"
      />
      {save.isError && <Alert tone="danger">{describeError(save.error)}</Alert>}
    </div>
  );
}

/** Browse, edit, create and delete files inside one website, without FTP or SSH. */
export function FileManager({ site }: { site: Site }) {
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState("");
  const [openFile, setOpenFile] = useState<string | null>(null);
  const [creating, setCreating] = useState<"folder" | "file" | null>(null);
  const [newName, setNewName] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const createFolder = useCreateFolder(site.id);
  const saveNew = useSaveFile(site.id);
  const [createError, setCreateError] = useState<string | null>(null);

  const submitNew = () => {
    const name = newName.trim();
    if (!name || name.includes("/") || name === "." || name === "..") {
      setCreateError("Use a plain name without slashes.");
      return;
    }
    setCreateError(null);
    const full = joinPath(path, name);
    if (creating === "folder") {
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
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setOpenFile(null);
          setCreating(null);
          setCreateError(null);
        }
      }}
    >
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <Folder aria-hidden /> Files
        </Button>
      </DialogTrigger>
      <DialogContent className="inset-0 top-0 h-dvh max-h-none w-full max-w-none translate-x-0 translate-y-0 rounded-none p-4 sm:inset-x-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-[min(96vw,980px)] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-lg sm:p-6">
        <DialogHeader>
          <DialogTitle>Files · {site.domain}</DialogTitle>
          <DialogDescription>
            Browse the website's folder. WordPress core files are protected from deletion.
          </DialogDescription>
        </DialogHeader>

        {openFile ? (
          <Editor siteId={site.id} path={openFile} onClose={() => setOpenFile(null)} />
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <Crumbs path={path} onGo={go} />
              <div className="ml-auto flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setCreating("folder");
                    setNewName("");
                    setCreateError(null);
                  }}
                >
                  <FolderPlus aria-hidden /> New folder
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
                  <FilePlus aria-hidden /> New file
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Refresh"
                  onClick={() => setRefreshKey((k) => k + 1)}
                >
                  <RefreshCw aria-hidden />
                </Button>
              </div>
            </div>

            {creating && (
              <form
                className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/30 p-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  submitNew();
                }}
              >
                <Input
                  autoFocus
                  className="w-full sm:max-w-xs"
                  placeholder={creating === "folder" ? "folder name" : "file name, e.g. notes.txt"}
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  aria-label={creating === "folder" ? "New folder name" : "New file name"}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={!newName.trim() || createFolder.isPending || saveNew.isPending}
                >
                  Create
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={() => setCreating(null)}>
                  Cancel
                </Button>
                {createError && (
                  <span className="text-xs text-destructive" role="alert">
                    {createError}
                  </span>
                )}
              </form>
            )}

            <Listing
              key={`${path}-${refreshKey}`}
              siteId={site.id}
              path={path}
              onOpenFolder={go}
              onOpenFile={setOpenFile}
            />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
