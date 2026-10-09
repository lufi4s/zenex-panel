import { Dialog as KobalteDialog } from "@kobalte/core/dialog";
import {
  children as resolveChildren,
  createContext,
  createSignal,
  useContext,
  type Accessor,
  type JSX,
} from "solid-js";
import { cn } from "@/lib/utils";

interface DialogContext {
  open: Accessor<boolean>;
  setOpen: (open: boolean) => void;
}

const Ctx = createContext<DialogContext>();

/**
 * A modal window (Kobalte: focus trap, Escape to close, accessible roles).
 * It works controlled (open + onOpenChange) or uncontrolled (the trigger opens it).
 */
export function Dialog(props: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: JSX.Element;
}) {
  const [internal, setInternal] = createSignal(false);
  const isOpen = () => props.open ?? internal();
  const setOpen = (next: boolean) => {
    if (props.open === undefined) setInternal(next);
    props.onOpenChange?.(next);
  };
  return (
    <Ctx.Provider value={{ open: isOpen, setOpen }}>
      <KobalteDialog open={isOpen()} onOpenChange={setOpen}>
        <DialogBody>{props.children}</DialogBody>
      </KobalteDialog>
    </Ctx.Provider>
  );
}

/** Renders the children once, inside Kobalte's context, so each part is built a single time. */
function DialogBody(props: { children: JSX.Element }) {
  const kids = resolveChildren(() => props.children);
  return <>{kids()}</>;
}

/**
 * Opens the dialog when the child is clicked. The wrapper has no ARIA role of its own,
 * so the child button stays the only control announced to assistive technology.
 */
export function DialogTrigger(props: { children: JSX.Element }) {
  const ctx = useContext(Ctx);
  return (
    <span class="contents" onClick={() => ctx?.setOpen(true)}>
      {props.children}
    </span>
  );
}

export function DialogContent(props: { class?: string; children: JSX.Element }) {
  return (
    <KobalteDialog.Portal>
      <KobalteDialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
      <KobalteDialog.Content
        class={cn(
          "fixed left-1/2 top-1/2 z-50 flex max-h-[90vh] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 flex-col gap-4 overflow-y-auto rounded-xl border border-border bg-card p-6 shadow-lg focus:outline-none",
          props.class,
        )}
      >
        {props.children}
      </KobalteDialog.Content>
    </KobalteDialog.Portal>
  );
}

export function DialogHeader(props: { class?: string; children: JSX.Element }) {
  return <div class={cn("flex flex-col gap-1.5", props.class)}>{props.children}</div>;
}

export function DialogTitle(props: { class?: string; children: JSX.Element }) {
  return (
    <KobalteDialog.Title
      class={cn("font-heading text-lg font-semibold leading-none tracking-tight", props.class)}
    >
      {props.children}
    </KobalteDialog.Title>
  );
}

export function DialogDescription(props: { class?: string; children: JSX.Element }) {
  return (
    <KobalteDialog.Description class={cn("text-sm text-muted-foreground", props.class)}>
      {props.children}
    </KobalteDialog.Description>
  );
}

export function DialogFooter(props: { class?: string; children: JSX.Element }) {
  return (
    <div class={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", props.class)}>
      {props.children}
    </div>
  );
}
