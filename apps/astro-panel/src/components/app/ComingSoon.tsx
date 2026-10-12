/** Stands in for a section that is rebuilt in a later phase. */
export function ComingSoon({ title }: { title: string }) {
  return (
    <div className="rounded-xl border border-dashed p-10 text-center">
      <p className="font-medium">{title} is being rebuilt.</p>
      <p className="mt-1 text-sm text-muted-foreground">Until then, use the current panel.</p>
    </div>
  );
}
