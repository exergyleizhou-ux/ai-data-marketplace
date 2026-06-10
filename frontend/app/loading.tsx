// Root loading UI, shown via Suspense during route segment navigation. Server
// component, deliberately text-light so it needs no i18n/providers.

export default function Loading() {
  return (
    <div className="mx-auto max-w-3xl animate-pulse space-y-4 py-6" aria-busy="true" aria-label="loading">
      <div className="h-7 w-1/3 rounded bg-neutral-200" />
      <div className="h-4 w-2/3 rounded bg-neutral-100" />
      <div className="space-y-3 pt-4">
        <div className="h-24 rounded-xl bg-neutral-100" />
        <div className="h-24 rounded-xl bg-neutral-100" />
        <div className="h-24 rounded-xl bg-neutral-100" />
      </div>
    </div>
  );
}
