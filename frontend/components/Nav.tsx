"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuth } from "@/lib/auth";
import { BRAND } from "@/lib/brand";
import { useT, LangToggle } from "@/lib/i18n";
import { api } from "@/lib/api";
import { Badge } from "./ui";

const LINKS = [
  { href: "/datasets", zh: "数据市场", en: "Marketplace" },
  { href: "/sell", zh: "我要卖", en: "Sell", auth: true },
  { href: "/compute", zh: "隐私计算", en: "Compute", auth: true },
  { href: "/orders", zh: "我的订单", en: "Orders", auth: true },
  { href: "/earnings", zh: "收益", en: "Earnings", auth: true },
];

export function Nav() {
  const { user, loading, logout } = useAuth();
  const { t } = useT();
  const pathname = usePathname();
  const router = useRouter();
  const isOps = user?.role === "ops" || user?.role === "admin";
  const [unread, setUnread] = useState(0);

  useEffect(() => {
    if (!user) return;
    api.countUnreadNotifications().then((r) => setUnread(r.unread)).catch(() => {});
  }, [user]);

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
              {t(l.zh, l.en)}
            </Link>
          ))}
          {isOps && (
            <Link
              href="/admin"
              className={`rounded-md px-3 py-1.5 text-sm ${
                pathname.startsWith("/admin") ? "bg-neutral-100 font-medium" : "text-neutral-600 hover:bg-neutral-50"
              }`}
            >
              {t("运营后台", "Ops")}
            </Link>
          )}
        </nav>
        <div className="flex items-center gap-3 text-sm">
          <LangToggle />
          {loading ? null : user ? (
            <>
              <Link
                href="/notifications"
                className="relative text-neutral-500 hover:text-neutral-900"
                title={t("通知", "Notifications")}
                aria-label={
                  unread > 0
                    ? t(`通知,${unread} 条未读`, `Notifications, ${unread} unread`)
                    : t("通知", "Notifications")
                }
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                  <path d="M13.73 21a2 2 0 0 1-3.46 0" />
                </svg>
                {unread > 0 && (
                  <span
                    role="status"
                    aria-live="polite"
                    className="absolute -right-1.5 -top-1.5 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold text-white"
                  >
                    {unread > 99 ? "99+" : unread}
                  </span>
                )}
              </Link>
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
                {t("退出", "Sign out")}
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className="text-neutral-600 hover:text-neutral-900">
                {t("登录", "Sign in")}
              </Link>
              <Link href="/register" className="rounded-md bg-neutral-900 px-3 py-1.5 text-white hover:bg-neutral-700">
                {t("注册", "Sign up")}
              </Link>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
