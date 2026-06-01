import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { Nav } from "@/components/Nav";
import { BRAND } from "@/lib/brand";

export const metadata: Metadata = {
  title: `${BRAND.name} — ${BRAND.tagline}`,
  description: `${BRAND.sloganZh} ${BRAND.description}`,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen bg-neutral-50 text-neutral-900 antialiased">
        <AuthProvider>
          <Nav />
          <main className="mx-auto max-w-6xl px-4 py-8">{children}</main>
          <footer className="mx-auto max-w-6xl px-4 py-8 text-sm text-neutral-500">
            <p className="font-medium text-neutral-700">{BRAND.name}</p>
            <p className="mt-1 italic">{BRAND.sloganEn}</p>
            <p>{BRAND.sloganZh}</p>
            <p className="mt-3">
              <Link href="/terms" className="hover:underline">
                用户服务协议
              </Link>
              <span className="mx-2">·</span>
              <Link href="/privacy" className="hover:underline">
                隐私政策
              </Link>
            </p>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
