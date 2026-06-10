"use client";

// 404 page. Rendered inside the root layout (providers available) for unmatched
// routes and any explicit notFound() call.

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { Card } from "@/components/ui";

export default function NotFound() {
  const { t } = useT();
  return (
    <div className="mx-auto max-w-md py-10">
      <Card className="text-center">
        <p className="text-4xl font-bold tracking-tight text-neutral-300">404</p>
        <h1 className="mt-2 text-lg font-semibold text-neutral-900">
          {t("页面不存在", "Page not found")}
        </h1>
        <p className="mt-2 text-sm text-neutral-600">
          {t("你访问的页面不存在或已被移除。", "The page you’re looking for doesn’t exist or has been moved.")}
        </p>
        <div className="mt-5">
          <Link
            href="/"
            className="inline-flex items-center rounded-md bg-neutral-900 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-700"
          >
            {t("返回首页", "Go home")}
          </Link>
        </div>
      </Card>
    </div>
  );
}
