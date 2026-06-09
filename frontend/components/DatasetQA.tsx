"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type DatasetQuestion } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Alert, Button, Card, Spinner } from "@/components/ui";

export function DatasetQA({ datasetId, sellerId }: { datasetId: string; sellerId: string }) {
  const { user } = useAuth();
  const { t } = useT();
  const [questions, setQuestions] = useState<DatasetQuestion[] | null>(null);
  const [askBody, setAskBody] = useState("");
  const [answerBodies, setAnswerBodies] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    try {
      const r = await api.listDatasetQuestions(datasetId);
      setQuestions(r.items);
    } catch {
      setQuestions([]);
    }
  }, [datasetId]);
  useEffect(() => { void load(); }, [load]);

  const isSeller = user?.id === sellerId;

  async function submitAsk() {
    if (!askBody.trim()) return;
    setBusy(true); setErr("");
    try {
      await api.askDatasetQuestion(datasetId, askBody.trim());
      setAskBody("");
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function submitAnswer(qid: string) {
    const body = (answerBodies[qid] || "").trim();
    if (!body) return;
    setBusy(true); setErr("");
    try {
      await api.answerQuestion(qid, body);
      setAnswerBodies((prev) => { const n = { ...prev }; delete n[qid]; return n; });
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  if (questions === null) return <Spinner />;

  return (
    <Card>
      <h3 className="mb-3 font-semibold">
        {t("数据集问答", "Dataset Q&A")} <span className="font-normal text-neutral-400">/ Q&A</span>
      </h3>
      {err && <Alert>{err}</Alert>}

      {user ? (
        <div className="mb-4 flex gap-2">
          <textarea
            className="flex-1 rounded-md border border-neutral-300 p-2 text-sm"
            rows={2}
            value={askBody}
            onChange={(e) => setAskBody(e.target.value)}
            placeholder={t("有问题想问卖家?", "Ask the seller a question...") as string}
          />
          <Button onClick={submitAsk} disabled={busy || !askBody.trim()}>
            {t("发布提问", "Post")}
          </Button>
        </div>
      ) : (
        <Alert>{t("登录后提问", "Sign in to ask questions")}</Alert>
      )}

      {questions.length === 0 ? (
        <p className="text-sm text-neutral-400">{t("暂无问答", "No questions yet")}</p>
      ) : (
        <ul className="space-y-3">
          {questions.map((q) => (
            <li key={q.id} className="rounded-md border border-neutral-100 p-3">
              <div className="flex items-center gap-2 text-xs text-neutral-400">
                <span>{q.asker_name || q.asker_id?.slice(0, 8)}</span>
                <span>·</span>
                <span>{q.created_at?.slice(0, 10)}</span>
              </div>
              <p className="mt-1 text-sm">{q.body}</p>

              {q.answer ? (
                <div className="ml-3 mt-2 border-l-2 border-green-300 pl-3">
                  <div className="text-xs text-neutral-400">
                    {t("卖家回答", "Seller answered")} · {q.answer.created_at?.slice(0, 10)}
                  </div>
                  <p className="mt-0.5 text-sm">{q.answer.body}</p>
                </div>
              ) : isSeller ? (
                <div className="ml-3 mt-2 flex gap-2">
                  <textarea
                    className="flex-1 rounded-md border border-neutral-200 p-1.5 text-sm"
                    rows={2}
                    value={answerBodies[q.id] || ""}
                    onChange={(e) => setAnswerBodies((prev) => ({ ...prev, [q.id]: e.target.value }))}
                    placeholder={t("回答这个问题...", "Answer this question...") as string}
                  />
                  <Button onClick={() => submitAnswer(q.id)} disabled={busy || !(answerBodies[q.id] || "").trim()}>
                    {t("回答", "Reply")}
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
