import Link from "next/link";
import { LEGAL_VERSIONS } from "@/lib/legal";

export const metadata = { title: "用户服务协议 - AI 训练数据交易市场" };

export default function TermsPage() {
  return (
    <article className="mx-auto max-w-3xl space-y-6">
      <h1 className="text-2xl font-semibold">用户服务协议</h1>
      <div className="rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">
        ⚠️ <strong>草案占位（{LEGAL_VERSIONS.terms}）</strong>。最终条款须经执业律师审核定稿后方可生效;
        当前内容仅用于产品占位与流程联调,不构成法律意见,亦不具法律效力。
      </div>
      <p className="text-sm text-neutral-600">
        本协议将约定平台与用户(买家/卖家)之间的权利义务。定稿前的结构大纲如下:
      </p>
      <ol className="list-decimal space-y-2 pl-6 text-sm text-neutral-700">
        <li>总则与协议接受</li>
        <li>账户注册与实名认证(KYC)</li>
        <li>平台定位:信息撮合与技术服务;资金由持牌方结算,平台不存管</li>
        <li>卖家义务:数据来源合法性声明与授权保证</li>
        <li>买家义务:在授权范围内使用,不得超范围使用或再分发</li>
        <li>交易、价格、佣金与税费</li>
        <li>交付、验收与纠纷处理</li>
        <li>知识产权、责任限制、违约处理</li>
        <li>法律适用与争议解决</li>
      </ol>
      <p className="text-sm text-neutral-500">
        完整草案见仓库 <code>docs/legal/用户服务协议-草案.md</code>。相关:{" "}
        <Link href="/privacy" className="font-medium text-neutral-900 hover:underline">
          隐私政策
        </Link>
        。
      </p>
    </article>
  );
}
