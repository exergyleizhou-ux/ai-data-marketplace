import type { Metadata } from "next";
import { Geist, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { LocaleProvider } from "@/lib/i18n";
import { Nav } from "@/components/Nav";
import { SiteFooter } from "@/components/SiteFooter";
import { BRAND } from "@/lib/brand";

// Design system fonts (see DESIGN.md):
// - Instrument Serif = display, signals editorial gravitas + cryptographic authority
// - Geist = body, precise + technical without feeling cold
// - Geist Mono = data, certificate IDs, hashes, sample counts (proof points)
const display = Instrument_Serif({
  subsets: ["latin"],
  weight: "400",
  variable: "--font-display",
  display: "swap",
});

const body = Geist({
  subsets: ["latin"],
  variable: "--font-body",
  display: "swap",
});

const mono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: `${BRAND.name} — ${BRAND.tagline}`,
  description: `${BRAND.sloganZh} ${BRAND.description}`,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" className={`${display.variable} ${body.variable} ${mono.variable}`}>
      <body className="min-h-screen bg-paper text-ink antialiased">
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
