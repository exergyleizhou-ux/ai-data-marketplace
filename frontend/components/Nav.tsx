"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { BRAND } from "@/lib/brand";
import { Badge } from "./ui";

const LINKS = [
  { href: "/datasets", label: "数据市场" },
  { href: "/sell", label: "我要卖", auth: true },
  { href: "/orders", label: "我的订单", auth: true },
  { href: "/earnings", label: "收益", auth: true },
];

export function Nav() {
  const { user, loading, logout } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  const isOps = user?.role === "ops" || user?.role === "admin";

  return (
    <header className="sticky top-0 z-10 border-b border-neutral-200 bg-white/80 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-6 px-4">
        <Link href="/" className="font-semibold tracking-tight">
          {BRAND.nameEn} <span className="text-neutral-400">{BRAND.nameZh}</span>
        </Link>
        <nav className="flex flex-1 items-center gap-1">
          {LINKS.filter((l) => !l.auth || user).map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className={`rounded-md px-3 py-1.5 text-sm ${
                pathname.startsWith(l.href) ? "bg-neutral-100 font-medium text-neutral-900" : "text-neutral-600 hover:bg-neutral-50"
              }`}
            >
              {l.label}
            </Link>
          ))}
          {isOps && (
            <Link
              href="/admin"
              className={`rounded-md px-3 py-1.5 text-sm ${
                pathname.startsWith("/admin") ? "bg-neutral-100 font-medium" : "text-neutral-600 hover:bg-neutral-50"
              }`}
            >
              运营后台
            </Link>
          )}
        </nav>
        <div className="flex items-center gap-3 text-sm">
          {loading ? null : user ? (
            <>
              <Link href="/account" className="flex items-center gap-2 text-neutral-700 hover:text-neutral-900">
                <span className="max-w-[10rem] truncate">{user.account}</span>
                <Badge>{user.kyc_status}</Badge>
              </Link>
              <button
                onClick={() => {
                  logout();
                  router.push("/");
                }}
                className="text-neutral-500 hover:text-neutral-900"
              >
                退出
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className="text-neutral-600 hover:text-neutral-900">
                登录
              </Link>
              <Link href="/register" className="rounded-md bg-neutral-900 px-3 py-1.5 text-white hover:bg-neutral-700">
                注册
              </Link>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
