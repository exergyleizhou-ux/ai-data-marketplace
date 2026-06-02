import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { LocaleProvider } from "@/lib/i18n";
import { Nav } from "@/components/Nav";
import { SiteFooter } from "@/components/SiteFooter";
import { BRAND } from "@/lib/brand";

export const metadata: Metadata = {
  title: `${BRAND.name} — ${BRAND.tagline}`,
  description: `${BRAND.sloganZh} ${BRAND.description}`,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen bg-neutral-50 text-neutral-900 antialiased">
        <LocaleProvider>
          <AuthProvider>
            <Nav />
            <main className="mx-auto max-w-6xl px-4 py-8">{children}</main>
            <SiteFooter />
          </AuthProvider>
        </LocaleProvider>
      </body>
    </html>
  );
}
