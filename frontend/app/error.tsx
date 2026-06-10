"use client";

// Segment-level error boundary. Next.js renders this (inside the root layout, so
// providers are available) whenever a Server/Client Component below it throws
// during render. `reset()` re-attempts the segment without a full reload.

import { useEffect } from "react";
import Link from "next/link";
import { useT } from "@/lib/i18n";
import { Button, Card } from "@/components/ui";

export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const { t } = useT();

  useEffect(() => {
    // Surface to the browser console / any client error reporter.
    console.error("page error boundary:", error);
  }, [error]);

  return (
    <div className="mx-auto max-w-md py-10">
      <Card className="text-center">
        <h1 className="text-lg font-semibold text-neutral-900">
          {t("出了点问题", "Something went wrong")}
        </h1>
        <p className="mt-2 text-sm text-neutral-600">
          {t(
            "页面加载时发生错误。可以重试,或返回首页。",
            "An error occurred while loading this page. You can retry or go home.",
          )}
        </p>
        {error.digest && (
          <p className="mt-2 font-mono text-xs text-neutral-400">ref: {error.digest}</p>
        )}
        <div className="mt-5 flex justify-center gap-3">
          <Button onClick={() => reset()}>{t("重试", "Try again")}</Button>
          <Link href="/" className="inline-flex items-center text-sm font-medium text-neutral-600 hover:underline">
            {t("返回首页", "Go home")}
          </Link>
        </div>
      </Card>
    </div>
  );
}
