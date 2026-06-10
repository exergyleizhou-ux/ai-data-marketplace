// Loading skeleton for a single dataset detail page.

export default function Loading() {
  return (
    <div className="mx-auto max-w-4xl animate-pulse space-y-6 py-2" aria-busy="true" aria-label="loading">
      <div className="space-y-3">
        <div className="h-8 w-2/3 rounded bg-neutral-200" />
        <div className="h-4 w-1/3 rounded bg-neutral-100" />
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="h-40 rounded-xl bg-neutral-100 md:col-span-2" />
        <div className="h-40 rounded-xl bg-neutral-100" />
      </div>
      <div className="h-32 rounded-xl bg-neutral-100" />
    </div>
  );
}
