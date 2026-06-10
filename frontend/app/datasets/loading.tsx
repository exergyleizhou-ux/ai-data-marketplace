// Loading skeleton for the dataset catalog (the list fetches on mount).

export default function Loading() {
  return (
    <div className="animate-pulse space-y-6 py-2" aria-busy="true" aria-label="loading">
      <div className="h-8 w-40 rounded bg-neutral-200" />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="space-y-3 rounded-xl border border-neutral-200 bg-white p-5">
            <div className="h-5 w-3/4 rounded bg-neutral-200" />
            <div className="h-4 w-full rounded bg-neutral-100" />
            <div className="h-4 w-2/3 rounded bg-neutral-100" />
            <div className="h-6 w-20 rounded-full bg-neutral-100" />
          </div>
        ))}
      </div>
    </div>
  );
}
