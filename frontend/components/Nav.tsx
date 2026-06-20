"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuth } from "@/lib/auth";
import { BRAND } from "@/lib/brand";
import { useT, LangToggle, kycLabel } from "@/lib/i18n";
import { api } from "@/lib/api";
import { Badge } from "./ui";

const LINKS = [
  { href: "/datasets", zh: "数据市场", en: "Marketplace" },
  { href: "/c2d", zh: "可信计算", en: "Compute-to-data" },
  { href: "/verify-api", zh: "验证 API", en: "Verify API" },
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
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    if (!user) return;
    api.countUnreadNotifications().then((r) => setUnread(r.unread)).catch(() => {});
  }, [user]);

  // Close menu on route change.
  useEffect(() => {
    setMenuOpen(false);
  }, [pathname]);

  const visibleLinks = LINKS.filter((l) => !l.auth || user);
  const linkClass = (href: string) =>
    `rounded-md px-3 py-1.5 text-sm ${
      pathname.startsWith(href) ? "bg-neutral-100 font-medium text-neutral-900" : "text-neutral-600 hover:bg-neutral-50"
    }`;
  // Mobile drawer rows get a comfortable ≥44px touch target.
  const drawerLinkClass = (href: string) =>
    `flex min-h-[44px] items-center rounded-md px-3 text-sm ${
      pathname.startsWith(href) ? "bg-neutral-100 font-medium text-neutral-900" : "text-neutral-700 hover:bg-neutral-50"
    }`;

  return (
    <header className="sticky top-0 z-10 border-b border-rule bg-paper/85 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:gap-6">
        {/* Hamburger (mobile only): toggles the link drawer below. */}
        <button
          type="button"
          onClick={() => setMenuOpen((v) => !v)}
          aria-label={t("菜单", "Menu")}
          aria-expanded={menuOpen}
          className="sm:hidden -ml-1.5 inline-flex h-11 w-11 items-center justify-center rounded-md text-neutral-600 hover:bg-neutral-100"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            {menuOpen ? <path d="M18 6L6 18M6 6l12 12" /> : <><path d="M3 6h18" /><path d="M3 12h18" /><path d="M3 18h18" /></>}
          </svg>
        </button>
        <Link href="/" className="whitespace-nowrap font-display text-2xl leading-none tracking-tight">
          {BRAND.nameEn}
          <span className="ml-1.5 hidden font-mono text-[10px] uppercase tracking-widest text-muted sm:inline-block sm:align-middle">
            {BRAND.nameZh}
          </span>
        </Link>
        {/* Desktop links */}
        <nav className="hidden flex-1 items-center gap-1 sm:flex">
          {visibleLinks.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className={linkClass(l.href)}
              aria-current={pathname.startsWith(l.href) ? "page" : undefined}
            >
              {t(l.zh, l.en)}
            </Link>
          ))}
          {isOps && (
            <Link href="/admin" className={linkClass("/admin")}>
              {t("运营后台", "Ops")}
            </Link>
          )}
        </nav>
        {/* Spacer keeps right cluster pinned on mobile (where nav above is hidden). */}
        <div className="flex-1 sm:hidden" />
        <div className="flex items-center gap-2 text-sm sm:gap-3">
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
              {/* Account: show email on desktop, only the kyc badge on mobile. */}
              <Link href="/account" className="flex items-center gap-2 text-neutral-700 hover:text-neutral-900">
                <span className="hidden max-w-[10rem] truncate sm:inline">{user.account}</span>
                <Badge>{kycLabel(user.kyc_status, t)}</Badge>
              </Link>
              <button
                onClick={() => {
                  logout();
                  router.push("/");
                }}
                className="text-neutral-500 hover:text-neutral-900 whitespace-nowrap"
              >
                {t("退出", "Sign out")}
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className="text-neutral-600 hover:text-neutral-900 whitespace-nowrap">
                {t("登录", "Sign in")}
              </Link>
              <Link href="/register" className="rounded-md bg-neutral-900 px-3 py-1.5 text-white hover:bg-neutral-700 whitespace-nowrap">
                {t("注册", "Sign up")}
              </Link>
            </>
          )}
        </div>
      </div>
      {/* Mobile drawer: shows when hamburger is open, hidden on sm+. */}
      {menuOpen && (
        <div className="border-t border-neutral-200 bg-white px-2 py-1 sm:hidden">
          <nav className="flex flex-col divide-y divide-rule/70">
            {visibleLinks.map((l) => (
              <Link
                key={l.href}
                href={l.href}
                className={drawerLinkClass(l.href)}
                aria-current={pathname.startsWith(l.href) ? "page" : undefined}
              >
                {t(l.zh, l.en)}
              </Link>
            ))}
            {isOps && (
              <Link href="/admin" className={drawerLinkClass("/admin")}>
                {t("运营后台", "Ops")}
              </Link>
            )}
          </nav>
        </div>
      )}
    </header>
  );
}
