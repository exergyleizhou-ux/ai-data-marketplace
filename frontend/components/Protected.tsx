"use client";

import Link from "next/link";
import { useAuth } from "@/lib/auth";
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
  const { user, loading } = useAuth();
  if (loading) return <Spinner />;
  if (!user)
    return (
      <Prompt title="请先登录">
        <Link href="/login" className="font-medium text-neutral-900 underline">
          去登录
        </Link>
      </Prompt>
    );
  if (requireOps && user.role !== "ops" && user.role !== "admin")
    return <Prompt title="需要运营权限">当前账号无权访问运营后台。</Prompt>;
  if (requireKYC && user.kyc_status !== "verified")
    return (
      <Prompt title="需要完成实名认证">
        买卖数据前必须通过实名认证。{" "}
        <Link href="/account" className="font-medium text-neutral-900 underline">
          去实名
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
