import type { Metadata } from "next";
import { BRAND, LegalHeader, LegalSection, P, List, LegalFooterNav } from "@/components/Legal";

export const metadata: Metadata = {
  title: "隐私政策 / Privacy Policy — Verdant Oasis",
  description: BRAND.philosophyZh,
};

export default function PrivacyPage() {
  return (
    <article className="mx-auto max-w-3xl space-y-10 py-2">
      <LegalHeader titleZh="隐私政策" titleEn="Privacy Policy" />

      <LegalSection n={1} zh="引言" en="Introduction">
        <P
          zh={`${BRAND.entity}（下称“我们”）深知个人信息对您的重要性。本《隐私政策》说明我们在您使用 Verdant Oasis（绿洲）平台时如何收集、使用、存储、共享与保护您的个人信息，以及您所享有的权利。`}
          en={`${BRAND.entityEn} ("we", "us") understands the importance of your personal information. This Privacy Policy explains how we collect, use, store, share, and protect your personal information when you use the Verdant Oasis platform, and the rights available to you.`}
        />
        <P
          zh="我们依据《个人信息保护法》《数据安全法》《网络安全法》等法律法规处理个人信息。请您仔细阅读本政策；勾选同意或使用本平台，即表示您理解并同意本政策。"
          en="We process personal information in accordance with the Personal Information Protection Law (PIPL), Data Security Law, Cybersecurity Law, and other applicable laws. Please read this Policy carefully; by consenting or using the Platform, you acknowledge and agree to it."
        />
      </LegalSection>

      <LegalSection n={2} zh="我们收集的个人信息" en="Personal Information We Collect">
        <P zh="我们仅在为实现具体功能所必需的范围内收集个人信息：" en="We collect personal information only to the extent necessary to deliver specific functions:" />
        <List
          items={[
            ["注册信息：手机号/邮箱、账户密码（加密存储）。", "Registration: mobile number/email, account password (stored encrypted)."],
            ["实名认证信息：姓名、证件号码、机构信息等，用于身份核验与合规准入。", "Real-name verification: name, ID number, organization details — for identity verification and compliant onboarding."],
            ["交易信息：订单记录、支付与结算信息、上架/购买的数据集记录。", "Transaction: order records, payment and settlement information, listing/purchase records."],
            ["设备与日志信息：IP 地址、设备标识、浏览器类型、访问日志，用于安全风控与服务运行。", "Device & logs: IP address, device identifiers, browser type, access logs — for security/risk control and service operation."],
          ]}
        />
        <P
          zh="我们不会主动收集与服务无关的敏感个人信息。如某项功能需处理敏感个人信息，我们将单独征得您的同意。"
          en="We do not proactively collect sensitive personal information unrelated to the service. Where a function requires processing sensitive personal information, we will obtain your separate consent."
        />
      </LegalSection>

      <LegalSection n={3} zh="处理目的与合法性基础" en="Purposes & Lawful Basis">
        <P zh="我们基于以下合法性基础处理您的个人信息（《个人信息保护法》第十三条）：" en="We process your personal information on the following lawful bases (PIPL Article 13):" />
        <List
          items={[
            ["为订立和履行您作为一方当事人的合约（如交易、交付、结算）所必需；", "necessary to conclude or perform a contract to which you are a party (e.g., transaction, delivery, settlement);"],
            ["为履行法定义务（如实名制、反洗钱、配合监管）所必需；", "necessary to perform statutory obligations (e.g., real-name requirements, anti-money-laundering, regulatory cooperation);"],
            ["在征得您同意的范围内，用于提升服务与安全保障；", "within the scope of your consent, to improve services and security;"],
            ["法律法规规定的其他情形。", "other circumstances provided by laws and regulations."],
          ]}
        />
      </LegalSection>

      <LegalSection n={4} zh="Cookie 与同类技术" en="Cookies & Similar Technologies">
        <P
          zh="我们使用 Cookie 及本地存储等技术维持登录态、保障安全并优化体验。您可通过浏览器设置管理或清除 Cookie，但这可能影响部分功能的使用。"
          en="We use Cookies and local storage to maintain login state, ensure security, and optimize experience. You may manage or clear Cookies via browser settings, which may affect some functionality."
        />
      </LegalSection>

      <LegalSection n={5} zh="个人信息的对外提供与委托处理" en="Sharing & Entrusted Processing">
        <P zh="为实现平台功能，我们可能在最小必要范围内向下列第三方提供或委托处理个人信息，并以协议约束其合规处理：" en="To deliver Platform functions, we may share with, or entrust processing to, the following third parties on a minimum-necessary basis, bound by agreements to ensure compliant processing:" />
        <List
          items={[
            [`${BRAND.custodianZh}：用于支付、分账与结算。`, `Fund-custody institution [name to be filled in once the actual payment institution is confirmed]: for payment, split settlement, and settlement.`],
            ["实名核验服务商：用于身份认证。", "Identity-verification providers: for real-name authentication."],
            ["云存储与基础设施服务商：用于数据集与系统数据的存储与运行。", "Cloud storage and infrastructure providers: for storing and operating Dataset and system data."],
            ["质量检测相关服务（如适用）：用于数据集质检。", "Quality-inspection-related services (if applicable): for Dataset inspection."],
            ["依法配合的司法或监管机关。", "Judicial or regulatory authorities, as required by law."],
          ]}
        />
        <P
          zh="除上述情形、获得您单独同意或法律法规另有规定外，我们不会向第三方提供您的个人信息。"
          en="Except as above, with your separate consent, or as otherwise required by law, we will not provide your personal information to third parties."
        />
      </LegalSection>

      <LegalSection n={6} zh="个人信息的存储" en="Storage of Personal Information">
        <P
          zh="您的个人信息存储于中华人民共和国境内。我们仅在实现处理目的所必需的最短期限内保存您的个人信息，法律法规另有规定的从其规定；超出保存期限的，我们将删除或匿名化处理。"
          en="Your personal information is stored within the territory of the People's Republic of China. We retain it only for the shortest period necessary to achieve the processing purpose, unless laws require otherwise; beyond the retention period, we delete or anonymize it."
        />
        <P
          zh="本平台原则上不向境外提供个人信息。如确需跨境提供，我们将依《个人信息保护法》履行单独同意、个人信息保护影响评估及法定路径（如安全评估、标准合同或认证）等要求。"
          en="As a rule, the Platform does not provide personal information overseas. Where cross-border transfer is genuinely necessary, we will fulfill PIPL requirements including separate consent, a personal-information protection impact assessment, and a statutory pathway (e.g., security assessment, standard contract, or certification)."
        />
      </LegalSection>

      <LegalSection n={7} zh="个人信息安全" en="Information Security">
        <P
          zh="我们采取符合行业标准的安全措施保护个人信息，包括传输与存储加密、访问控制、最小权限、操作留痕与安全审计。但请注意，互联网环境并非绝对安全。"
          en="We adopt industry-standard security measures, including encryption in transit and at rest, access control, least-privilege, audit logging, and security audits. Please note that no internet environment is absolutely secure."
        />
        <P
          zh="如发生个人信息安全事件，我们将依法采取补救措施，并按规定向您和监管部门告知。"
          en="In the event of a personal-information security incident, we will take remedial measures and notify you and the authorities as required by law."
        />
      </LegalSection>

      <LegalSection n={8} zh="您的权利" en="Your Rights">
        <P zh="在法律法规规定的范围内，您对自己的个人信息享有以下权利：" en="Within the scope provided by law, you have the following rights over your personal information:" />
        <List
          items={[
            ["查阅、复制您的个人信息；", "access and copy your personal information;"],
            ["更正、补充不准确或不完整的个人信息；", "correct or supplement inaccurate or incomplete information;"],
            ["删除符合法定情形的个人信息；", "delete personal information where statutory conditions are met;"],
            ["撤回您此前作出的同意；", "withdraw consent previously given;"],
            ["注销账户；", "deregister your account;"],
            ["在符合条件时请求将个人信息转移至您指定的处理者。", "request transfer of your personal information to a designated handler where conditions are met."],
          ]}
        />
        <P
          zh={`您可通过 ${BRAND.dpoEmail} 行使上述权利。我们将在法定期限内核验并响应您的请求。`}
          en={`You may exercise these rights via ${BRAND.dpoEmail}. We will verify and respond within the statutory time limit.`}
        />
      </LegalSection>

      <LegalSection n={9} zh="未成年人保护" en="Protection of Minors">
        <P
          zh="本平台面向具备完全民事行为能力的用户，不面向未成年人。我们不会在明知的情况下收集未成年人的个人信息；如发现，将依法及时删除。"
          en="The Platform is intended for users with full civil capacity and is not directed at minors. We do not knowingly collect minors' personal information; if discovered, we will delete it promptly as required by law."
        />
      </LegalSection>

      <LegalSection n={10} zh="第三方链接与服务" en="Third-Party Links & Services">
        <P
          zh="本平台可能包含第三方链接或服务，其隐私实践由该第三方负责，本政策不适用于第三方。建议您在使用前查阅其隐私政策。"
          en="The Platform may contain third-party links or services whose privacy practices are their own responsibility; this Policy does not apply to them. We recommend reviewing their privacy policies before use."
        />
      </LegalSection>

      <LegalSection n={11} zh="本政策的更新" en="Updates to this Policy">
        <P
          zh="我们可能适时更新本政策。对于重大变更，我们将以显著方式（如公告或站内通知）告知。更新后您继续使用本平台的，视为接受更新后的政策。"
          en="We may update this Policy from time to time. For material changes, we will notify you prominently (e.g., announcement or in-app notice). Continued use after an update constitutes acceptance."
        />
      </LegalSection>

      <LegalSection n={12} zh="个人信息保护负责人与联系方式" en="Contact & Person in Charge of PI Protection">
        <P
          zh={`如您对本政策或个人信息处理有任何疑问、意见或投诉，可联系我们的个人信息保护负责人：${BRAND.dpoEmail}。运营主体：${BRAND.entity}。`}
          en={`For any questions, comments, or complaints regarding this Policy or our processing of personal information, contact our person in charge of personal-information protection: ${BRAND.dpoEmail}. Operator: ${BRAND.entityEn}.`}
        />
        <P
          zh="如您认为我们的处理损害了您的合法权益，且与我们协商未果，您有权向网信、市场监管等有权部门投诉或举报。"
          en="If you believe our processing harms your lawful rights and we fail to resolve it, you may complain or report to competent authorities such as the cyberspace administration or market regulator."
        />
      </LegalSection>

      <LegalFooterNav current="privacy" />
    </article>
  );
}
