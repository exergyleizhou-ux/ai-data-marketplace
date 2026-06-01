import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { Nav } from "@/components/Nav";

export const metadata: Metadata = {
  title: "AI 训练数据交易市场",
  description: "高信任、可追溯、合规的 AI 训练数据流通平台",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen bg-neutral-50 text-neutral-900 antialiased">
        <AuthProvider>
          <Nav />
          <main className="mx-auto max-w-6xl px-4 py-8">{children}</main>
          <footer className="mx-auto max-w-6xl px-4 py-8 text-sm text-neutral-500">
            <Link href="/terms" className="hover:underline">
              用户服务协议
            </Link>
            <span className="mx-2">·</span>
            <Link href="/privacy" className="hover:underline">
              隐私政策
            </Link>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
