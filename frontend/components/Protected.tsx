"use client";

import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Spinner } from "./ui";

// Protected gates a page behind login. Optionally requires a verified KYC
// status or an ops role, showing a friendly prompt instead of the content.
export function Protected({
  children,
  requireKYC = false,
  requireOps = false,
}: {
  children: React.ReactNode;
  requireKYC?: boolean;
  requireOps?: boolean;
}) {
  const { t } = useT();
  const { user, loading } = useAuth();
  if (loading) return <Spinner label={t("加载中…", "Loading…")} />;
  if (!user)
    return (
      <Prompt title={t("请先登录", "Please sign in first")}>
        <Link href="/login" className="font-medium text-neutral-900 underline">
          {t("去登录", "Sign in")}
        </Link>
      </Prompt>
    );
  if (requireOps && user.role !== "ops" && user.role !== "admin")
    return (
      <Prompt title={t("需要运营权限", "Operator access required")}>
        {t("当前账号无权访问运营后台。", "This account has no admin access.")}
      </Prompt>
    );
  if (requireKYC && user.kyc_status !== "verified")
    return (
      <Prompt title={t("需要完成实名认证", "KYC verification required")}>
        {t("买卖数据前必须通过实名认证。", "KYC is required to buy or sell data.")}{" "}
        <Link href="/account" className="font-medium text-neutral-900 underline">
          {t("去实名", "Submit KYC")}
        </Link>
      </Prompt>
    );
  return <>{children}</>;
}

function Prompt({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mx-auto max-w-md rounded-xl border border-neutral-200 bg-white p-8 text-center">
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-2 text-sm text-neutral-600">{children}</p>
    </div>
  );
}
