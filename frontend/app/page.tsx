export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col justify-center gap-6 px-6 py-16">
      <h1 className="text-3xl font-semibold tracking-tight">
        AI 训练数据交易市场
      </h1>
      <p className="text-base leading-relaxed text-neutral-600">
        高信任、可追溯、合规的训练数据流通基础设施。质量可信 · 来源合规 ·
        资金安全。
      </p>
      <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-4 text-sm text-neutral-500">
        PR-01 脚手架 · 后端 <code>/api/v1</code> 与各业务模块将在后续 PR 接入。
      </div>
    </main>
  );
}
