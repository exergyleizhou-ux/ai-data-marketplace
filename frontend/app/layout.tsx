import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "AI 训练数据交易市场",
  description: "高信任、可追溯、合规的 AI 训练数据流通平台",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body className="antialiased">{children}</body>
    </html>
  );
}
