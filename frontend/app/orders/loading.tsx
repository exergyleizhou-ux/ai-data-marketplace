// Loading skeleton for the orders list (fetches on mount).

export default function Loading() {
  return (
    <div className="mx-auto max-w-3xl animate-pulse space-y-4 py-2" aria-busy="true" aria-label="loading">
      <div className="h-8 w-32 rounded bg-neutral-200" />
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="flex items-center justify-between rounded-xl border border-neutral-200 bg-white p-4">
          <div className="space-y-2">
            <div className="h-4 w-48 rounded bg-neutral-200" />
            <div className="h-3 w-32 rounded bg-neutral-100" />
          </div>
          <div className="h-6 w-16 rounded-full bg-neutral-100" />
        </div>
      ))}
    </div>
  );
}
