import {
  cloneElement,
  createContext,
  isValidElement,
  useContext,
  useEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactElement,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";

interface DialogContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
}

const DialogContext = createContext<DialogContextValue | null>(null);

function useDialog() {
  const ctx = useContext(DialogContext);
  if (!ctx) throw new Error("Dialog parts must be used inside <Dialog>");
  return ctx;
}

/** A modal window. Works controlled (open + onOpenChange) or uncontrolled (the trigger opens it). */
export function Dialog({
  open: controlled,
  onOpenChange,
  children,
}: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: ReactNode;
}) {
  const [internal, setInternal] = useState(false);
  const open = controlled ?? internal;
  const setOpen = (next: boolean) => {
    if (controlled === undefined) setInternal(next);
    onOpenChange?.(next);
  };
  return <DialogContext.Provider value={{ open, setOpen }}>{children}</DialogContext.Provider>;
}

/** Opens the dialog. With asChild, the click handler is added to its single child. */
export function DialogTrigger({
  asChild = false,
  children,
}: {
  asChild?: boolean;
  children: ReactNode;
}) {
  const { setOpen } = useDialog();
  if (asChild && isValidElement(children)) {
    const child = children as ReactElement<{ onClick?: (event: unknown) => void }>;
    return cloneElement(child, {
      onClick: (event: unknown) => {
        child.props.onClick?.(event);
        setOpen(true);
      },
    });
  }
  return (
    <button type="button" onClick={() => setOpen(true)}>
      {children}
    </button>
  );
}

export function DialogContent({ className, children }: HTMLAttributes<HTMLDivElement>) {
  const { open, setOpen } = useDialog();
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    panel.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // setOpen changes identity on each render; the listener only needs the open state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        aria-hidden="true"
        className="fixed inset-0 bg-black/50"
        onClick={() => setOpen(false)}
      />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        className={cn(
          "relative flex max-h-[90vh] w-full max-w-md flex-col gap-4 overflow-y-auto rounded-xl border border-border bg-card p-6 shadow-lg focus:outline-none",
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}

export function DialogHeader({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-1.5", className)} {...props} />;
}

export function DialogTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h2
      className={cn("font-heading text-lg font-semibold leading-none tracking-tight", className)}
      {...props}
    />
  );
}

export function DialogDescription({ className, ...props }: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn("text-sm text-muted-foreground", className)} {...props} />;
}

export function DialogFooter({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    />
  );
}
