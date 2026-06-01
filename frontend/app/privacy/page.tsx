import Link from "next/link";
import { LEGAL_VERSIONS } from "@/lib/legal";

export const metadata = { title: "隐私政策 - AI 训练数据交易市场" };

export default function PrivacyPage() {
  return (
    <article className="mx-auto max-w-3xl space-y-6">
      <h1 className="text-2xl font-semibold">隐私政策</h1>
      <div className="rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">
        ⚠️ <strong>草案占位（{LEGAL_VERSIONS.privacy}）</strong>。最终条款须经执业律师审核定稿后方可生效;
        当前内容仅用于产品占位与流程联调,不构成法律意见,亦不具法律效力。
      </div>
      <p className="text-sm text-neutral-600">
        本政策将依据《个人信息保护法》《数据安全法》说明个人信息的收集与处理。定稿前的结构大纲如下:
      </p>
      <ol className="list-decimal space-y-2 pl-6 text-sm text-neutral-700">
        <li>收集的个人信息(注册、实名、交易、使用日志)及目的</li>
        <li>敏感信息处理:证件号不明文存储,以带密钥哈希保存</li>
        <li>使用、共享与委托处理(含向持牌支付机构提供必要交易信息)</li>
        <li>存储地点(境内)与期限;跨境策略(P3 默认不出境)</li>
        <li>安全措施:加密、访问控制、审计、限流</li>
        <li>您的权利:查阅/复制/更正/删除/撤回同意/注销</li>
        <li>未成年人保护、Cookie、政策更新与联系方式</li>
      </ol>
      <p className="text-sm text-neutral-500">
        完整草案见仓库 <code>docs/legal/隐私政策-草案.md</code>。相关:{" "}
        <Link href="/terms" className="font-medium text-neutral-900 hover:underline">
          用户服务协议
        </Link>
        。
      </p>
    </article>
  );
}
