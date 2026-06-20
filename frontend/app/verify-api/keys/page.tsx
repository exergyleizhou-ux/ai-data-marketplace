"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, type ApiKey } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { PageHeader, Card, Button, Input, Badge, Spinner, Empty } from "@/components/ui";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api/v1";

export default function VerifyKeysPage() {
  const { user } = useAuth();
  const { t } = useT();
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const [newKey, setNewKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setKeys((await api.listApiKeys()).items ?? []);
    } catch {
      setKeys([]);
    }
  }, []);
  useEffect(() => {
    if (user) void load();
  }, [user, load]);

  async function create() {
    if (creating) return;
    setCreating(true);
    try {
      const res = await api.createApiKey(name.trim() || "default", "free");
      setNewKey(res.key);
      setName("");
      await load();
    } finally {
      setCreating(false);
    }
  }

  async function revoke(id: string) {
    await api.revokeApiKey(id);
    await load();
  }

  if (!user) {
    return (
      <div className="max-w-2xl space-y-4 pt-10">
        <Card>
          <p className="text-sm text-ink/80">{t("请先登录以管理你的 Verify API 密钥。", "Please log in to manage your Verify API keys.")}</p>
          <Link href="/login" className="mt-3 inline-block font-medium text-forest-700 hover:underline">{t("去登录 →", "Log in →")}</Link>
        </Card>
      </div>
    );
  }

  return (
    <div className="max-w-2xl space-y-6 pb-20 pt-2">
      <PageHeader
        kicker={t("Oasis Verify", "Oasis Verify")}
        title={t("API 密钥", "API keys")}
        subtitle={t("用密钥调用验证 API:把数据集 POST 到 /screen,拿回报告 + 可验证证书。密钥只在创建时显示一次。", "Use a key to call the Verify API: POST a dataset to /screen and get a report + a verifiable certificate. Each key is shown only once, at creation.")}
      />

      {/* Create */}
      <Card className="space-y-3">
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-[12rem] flex-1">
            <label className="text-xs text-muted">{t("名称(可选)", "Name (optional)")}</label>
            <Input placeholder={t("如:my-laptop", "e.g. my-laptop")} value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <Button onClick={create} disabled={creating}>{creating ? t("创建中…", "Creating…") : t("创建密钥", "Create key")}</Button>
        </div>
        {newKey && (
          <div className="rounded-lg border border-forest-200 bg-forest-50/50 p-3">
            <p className="text-xs font-medium text-forest-800">{t("新密钥(只显示这一次,请立即保存):", "Your new key (shown once — copy it now):")}</p>
            <div className="mt-2 flex items-center gap-2">
              <code className="min-w-0 flex-1 break-all rounded bg-ink/90 px-2 py-1.5 font-mono text-xs text-paper">{newKey}</code>
              <Button
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(newKey);
                    setCopied(true);
                    setTimeout(() => setCopied(false), 1400);
                  } catch {
                    /* ignore */
                  }
                }}
              >
                {copied ? t("已复制 ✓", "Copied ✓") : t("复制", "Copy")}
              </Button>
            </div>
          </div>
        )}
      </Card>

      {/* Usage hint */}
      <pre className="overflow-x-auto rounded-xl bg-ink/95 p-4 text-[12px] leading-relaxed text-paper">
        <code>{`curl -X POST ${API_BASE}/screen \\
  -H "X-API-Key: <your key>" \\
  -F "file=@your-dataset.csv"`}</code>
      </pre>

      {/* List */}
      <section className="space-y-2">
        <h2 className="font-display text-lg text-ink">{t("你的密钥", "Your keys")}</h2>
        {keys === null ? (
          <Spinner />
        ) : keys.length === 0 ? (
          <Empty>{t("还没有密钥。创建一个开始吧。", "No keys yet. Create one to get started.")}</Empty>
        ) : (
          <ul className="space-y-2">
            {keys.map((k) => (
              <li key={k.id}>
                <Card className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <code className="font-mono text-sm text-ink">{k.prefix}…</code>
                      <Badge>{k.tier}</Badge>
                      {k.revoked_at && <Badge>{t("已吊销", "revoked")}</Badge>}
                    </div>
                    <p className="mt-0.5 text-xs text-muted">
                      {k.name || t("(未命名)", "(unnamed)")} · {t(`本月用量 ${k.usage_count}`, `${k.usage_count} scans this month`)}
                    </p>
                  </div>
                  {!k.revoked_at && (
                    <button onClick={() => revoke(k.id)} className="text-xs font-medium text-red-600 hover:underline">
                      {t("吊销", "Revoke")}
                    </button>
                  )}
                </Card>
              </li>
            ))}
          </ul>
        )}
      </section>

      <Link href="/verify-api" className="text-sm font-medium text-forest-700 hover:underline">{t("← 返回 Verify 产品页", "← Back to the Verify product page")}</Link>
    </div>
  );
}
