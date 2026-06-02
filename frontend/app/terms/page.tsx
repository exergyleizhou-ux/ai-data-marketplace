import type { Metadata } from "next";
import { BRAND, LegalHeader, LegalSection, P, List, LegalFooterNav } from "@/components/Legal";

export const metadata: Metadata = {
  title: "用户服务协议 / Terms of Service — Verdant Oasis",
  description: BRAND.philosophyZh,
};

export default function TermsPage() {
  return (
    <article className="mx-auto max-w-3xl space-y-10 py-2">
      <LegalHeader titleZh="用户服务协议" titleEn="Terms of Service" />

      <LegalSection n={1} zh="导言与协议接受" en="Introduction & Acceptance">
        <P
          zh={`欢迎使用 Verdant Oasis（绿洲）AI 训练数据交易平台（下称“本平台”）。本平台由 ${BRAND.entity}（下称“我们”或“平台方”）运营。本《用户服务协议》（下称“本协议”）是您与平台方之间就使用本平台服务所订立的具有法律约束力的协议。`}
          en={`Welcome to Verdant Oasis, an AI training-data marketplace (the "Platform"), operated by ${BRAND.entityEn} ("we", "us", or the "Operator"). These Terms of Service (the "Terms") form a legally binding agreement between you and the Operator governing your use of the Platform.`}
        />
        <P
          zh="您勾选同意、注册账户或以任何方式使用本平台，即表示您已阅读、理解并同意接受本协议全部条款。若您不同意，请勿使用本平台。"
          en="By checking the consent box, registering an account, or otherwise using the Platform, you acknowledge that you have read, understood, and agreed to be bound by all of these Terms. If you do not agree, do not use the Platform."
        />
        <P
          zh="若您代表机构使用本平台，您声明并保证已获得该机构的充分授权，本协议对该机构具有约束力。"
          en="If you use the Platform on behalf of an organization, you represent and warrant that you are duly authorized to bind that organization, and these Terms bind that organization."
        />
      </LegalSection>

      <LegalSection n={2} zh="定义" en="Definitions">
        <List
          items={[
            ["“用户”：指注册并使用本平台的个人或机构，包括数据卖方与数据买方。", '"User": any individual or organization that registers and uses the Platform, including data Sellers and data Buyers.'],
            ["“数据集”：指卖方在本平台上架、用于人工智能训练等用途的数据产品。", '"Dataset": a data product listed by a Seller on the Platform for purposes such as AI training.'],
            ["“来源声明”：指卖方就数据集的合法来源、授权链条与合规状态所作的承诺与举证。", '"Provenance Declaration": the Seller\'s representations and supporting evidence regarding a Dataset\'s lawful origin, chain of authorization, and compliance status.'],
            [`“分账/存管”：指交易资金不进入平台自有账户，由${BRAND.custodianZh}按指令存管与结算的资金处理模式。`, `"Split Settlement / Custody": a fund-handling model in which transaction funds do not enter the Platform's own account but are held and settled by ${BRAND.custodianEn} per instruction.`],
          ]}
        />
      </LegalSection>

      <LegalSection n={3} zh="账户注册与实名认证" en="Registration & Real-Name Verification">
        <P
          zh="本平台仅向中华人民共和国境内、已完成实名认证的用户开放。您应提供真实、准确、完整的注册与实名信息，并在信息变更时及时更新。"
          en="The Platform is open only to real-name-verified Users within the mainland territory of the People's Republic of China. You must provide true, accurate, and complete registration and identity information and keep it current."
        />
        <P
          zh="您应妥善保管账户凭证，对账户项下的全部行为负责。如发现账户被未经授权使用，应立即通知平台方。"
          en="You are responsible for safeguarding your credentials and for all activity under your account. Notify us immediately of any unauthorized use."
        />
        <P
          zh="平台方有权依法对用户身份进行核验。提供虚假信息或未通过实名认证的，平台方有权拒绝、限制或终止提供服务。"
          en="We may verify User identity as required by law. We may refuse, restrict, or terminate service for false information or failure to pass real-name verification."
        />
      </LegalSection>

      <LegalSection n={4} zh="平台角色与服务性质" en="Platform Role & Nature of Service">
        <P
          zh="本平台是为买卖双方提供信息撮合、质量检测、合约签订与资金存管支持的技术服务平台。除另有明确约定外，平台方不是数据交易的买方或卖方，亦不对数据集的内容、质量、权属作出担保。"
          en="The Platform is a technology service that provides matchmaking, quality inspection, contracting, and fund-custody support between Buyers and Sellers. Unless expressly stated otherwise, the Operator is not a party to any Dataset transaction and does not warrant the content, quality, or title of any Dataset."
        />
        <P
          zh="数据交易的权利义务由买卖双方通过本平台签订的交易合约确定。平台方提供的质检、形式审查等措施旨在提升交易信任，不构成对合规性或适用性的保证。"
          en="The rights and obligations of a Dataset transaction are governed by the transaction contract entered into by Buyer and Seller via the Platform. Quality inspection and formal review provided by us aim to enhance trust and do not constitute a guarantee of compliance or fitness for purpose."
        />
      </LegalSection>

      <LegalSection n={5} zh="数据上架与来源合规" en="Dataset Listing & Provenance Compliance">
        <P
          zh="卖方上架数据集前，必须就数据来源作出真实、完整的来源声明，并签署相关合规承诺。卖方应保证："
          en="Before listing a Dataset, the Seller must make a true and complete Provenance Declaration and sign the related compliance undertakings. The Seller warrants that:"
        />
        <List
          items={[
            ["对数据集享有合法权利或已获得充分授权，可在本平台流通与许可使用；", "it holds lawful rights to, or sufficient authorization for, the Dataset to be circulated and licensed on the Platform;"],
            ["数据的收集、处理符合《个人信息保护法》《数据安全法》《网络安全法》等法律法规；", "the collection and processing of the data complies with the Personal Information Protection Law (PIPL), Data Security Law, Cybersecurity Law, and other applicable laws;"],
            ["数据集中如包含个人信息，已取得必要的同意或具备其他合法性基础，并已做必要的去标识化/匿名化处理；", "where the Dataset contains personal information, the necessary consent or other lawful basis has been obtained, and necessary de-identification/anonymization has been performed;"],
            ["数据不含国家秘密、未经授权的商业秘密、违法或侵权内容。", "the data contains no state secrets, unauthorized trade secrets, or unlawful or infringing content."],
          ]}
        />
        <P
          zh="平台方对来源声明进行形式审查并全程留痕，但形式审查不免除卖方的合规与担保责任。因卖方来源不实导致的责任由卖方承担。"
          en="We conduct a formal (non-substantive) review of Provenance Declarations with full audit logging. Such review does not relieve the Seller of its compliance and warranty obligations. Liability arising from inaccurate provenance rests with the Seller."
        />
      </LegalSection>

      <LegalSection n={5.1} zh="AI 训练数据特别声明" en="Special Statement on AI Training Data">
        <P
          zh="用户理解并同意：本平台上架或交易的数据集可能被用于人工智能模型训练。卖方承诺并保证其数据集不包含国家秘密、未经充分去标识化的个人敏感信息、或可能引发算法偏见、歧视、伦理风险的内容。平台方不对买方使用数据集所产生的任何下游法律风险、伦理风险或模型训练结果承担责任。"
          en="Users understand and agree that Datasets listed or traded on the Platform may be used for training artificial-intelligence models. The Seller undertakes and warrants that its Dataset contains no state secrets, no sensitive personal information that has not been adequately de-identified, and no content that may give rise to algorithmic bias, discrimination, or ethical risk. The Operator bears no liability for any downstream legal risk, ethical risk, or model-training outcome arising from the Buyer's use of a Dataset."
        />
      </LegalSection>

      <LegalSection n={6} zh="质量检测" en="Quality Inspection">
        <P
          zh="数据集上传后将进入异步质检流程，包括格式校验、统计分析、去重检测与个人信息（PII）扫描。质检结果作为买方决策参考。"
          en="Uploaded Datasets undergo asynchronous quality inspection, including format validation, statistical analysis, de-duplication, and personal-information (PII) scanning. Inspection results serve as a reference for Buyers."
        />
        <P
          zh="质检为自动化辅助手段，可能存在局限。买方应自行评估数据集是否满足其用途。"
          en="Inspection is an automated aid with inherent limitations. Buyers should independently assess whether a Dataset meets their intended use."
        />
      </LegalSection>

      <LegalSection n={7} zh="交易、支付与资金存管" en="Transactions, Payment & Fund Custody">
        <P
          zh={`买方下单后，订单进入严格状态机管理（创建、已支付、已交付、已确认、已结算等）。交易资金不进入平台自有账户，由${BRAND.custodianZh}按本协议与交易指令进行存管与分账结算。`}
          en={`Once a Buyer places an order, it is managed by a strict state machine (created, paid, delivered, confirmed, settled, etc.). Transaction funds do not enter the Platform's own account; ${BRAND.custodianEn} holds and settles them via split settlement per these Terms and the transaction instructions.`}
        />
        <P
          zh="买方确认收货后，资金按约定结算给卖方（扣除平台服务费）。平台服务费的费率与计费方式以下单时页面展示为准。"
          en="Upon the Buyer's confirmation of receipt, funds are settled to the Seller per agreement (net of Platform service fees). Fee rates and methods are as displayed at the time of ordering."
        />
      </LegalSection>

      <LegalSection n={8} zh="交付、许可与使用限制" en="Delivery, License & Usage Restrictions">
        <P
          zh="数据集通过临时下载链接交付，并施加交付指纹以支持溯源。买方获得的是交易合约约定范围内的使用许可，而非数据的所有权。"
          en="Datasets are delivered via temporary download links with delivery fingerprints to support traceability. The Buyer obtains a license within the scope of the transaction contract, not ownership of the data."
        />
        <P
          zh="除许可范围明确允许外，买方不得转售、再许可、公开传播或超范围使用所购数据集。"
          en="Except as expressly permitted by the license scope, the Buyer may not resell, sub-license, publicly distribute, or use the purchased Dataset beyond the granted scope."
        />
      </LegalSection>

      <LegalSection n={9} zh="「可用不可见」沙箱计算服务" en={`Sandbox Compute ("Available-but-Invisible") Services`}>
        <P
          zh="除下载型交易外，本平台提供「可用不可见 / 沙箱计算」服务：买方在平台提供的隔离环境内，对数据集运行经平台审核的算法，仅取得计算结果（如模型、统计或查询结果），而不获得原始数据本身。"
          en={`In addition to download transactions, the Platform offers "available-but-invisible / sandbox compute" services: the Buyer runs platform-reviewed algorithms against a Dataset inside an isolated environment provided by the Platform and obtains only the computation output (e.g., a model, statistics, or query results), not the raw data itself.`}
        />
        <P
          zh="在沙箱计算交易中，平台不向买方交付原始数据；买方所得为计算输出物。"
          en="In a sandbox compute transaction, the Platform does not deliver raw data to the Buyer; the Buyer receives only the computation output."
        />
        <List
          items={[
            [
              "信任级别如实界定：L1（数据沙箱）下买方不可见原始数据，但平台运营方为运行沙箱仍可访问数据；仅 L2（机密计算 / TEE）对平台亦不可见。平台不就「可用不可见」作出超出所标信任级别的承诺。",
              "Honest trust-level scope: at L1 (data sandbox) the raw data is invisible to the Buyer, but the Platform operator can still access it in order to run the sandbox; only at L2 (confidential computing / TEE) is the data invisible to the Platform as well. The Platform makes no representation about \"available-but-invisible\" beyond the labeled trust level.",
            ],
            [
              "算法责任：买方仅可在平台审核通过的算法范围内提交作业，并对其所选算法的合法性、安全性及用途合规性负责；不得借算法或输出实施数据窃取、对个人信息再识别，或规避本协议。",
              "Algorithm responsibility: the Buyer may submit jobs only within the set of platform-approved algorithms and is responsible for the legality, safety, and compliant purpose of the chosen algorithm; the Buyer must not use an algorithm or its output to exfiltrate data, re-identify personal information, or circumvent these Terms.",
            ],
            [
              "输出物权属与使用：除另有约定外，计算输出物（如模型、指标）在合法合规前提下供买方在约定范围内使用；买方不得依据输出反向重建、推断或再识别原始数据或其中的个人信息。",
              "Output ownership & use: unless otherwise agreed, the computation output (e.g., model, metrics) may be used by the Buyer within the agreed scope, subject to law; the Buyer must not reconstruct, infer, or re-identify the raw data or any personal information therein from the output.",
            ],
            [
              "计费与不保证：计算权益按订单购买，并按成功放行核销；因平台或算法原因失败、或被输出闸门拒绝的作业按平台规则处理（通常返还相应额度）。平台不对计算结果的准确性、适用性或商业价值作任何明示或默示保证。",
              "Billing & no warranty: compute entitlements are purchased per order and consumed upon successful release; jobs that fail due to the Platform or the algorithm, or that are rejected by the output gate, are handled per Platform rules (typically the credit is refunded). The Platform makes no express or implied warranty as to the accuracy, fitness, or commercial value of any computation result.",
            ],
            [
              "留痕审计：为合规与纠纷处理，平台对计算作业全过程留痕（算法及版本、数据集版本、差分隐私预算消耗、放行 / 拒绝等）。",
              "Audit trail: for compliance and dispute handling, the Platform logs the full lifecycle of each compute job (algorithm and version, dataset version, differential-privacy budget spent, release/rejection, etc.).",
            ],
          ]}
        />
        <P
          zh="本服务的隐私保护程度以所标信任级别为准。平台秉持「信号非结论、不夸大」的一贯立场如实披露，具体以相应商品页说明为准。"
          en={`The degree of privacy protection of this service is governed by its labeled trust level. Consistent with our "signals, not verdicts — no overstatement" stance, the Platform discloses this honestly; the applicable product page prevails.`}
        />
      </LegalSection>

      <LegalSection n={10} zh="知识产权" en="Intellectual Property">
        <P
          zh="本平台的软件、界面、商标（包括 “Verdant Oasis / 绿洲” 标识）、Slogan 及相关内容的知识产权归平台方或权利人所有，未经授权不得使用。"
          en="Intellectual property in the Platform's software, interface, trademarks (including the “Verdant Oasis” mark), Slogan, and related content belongs to the Operator or its licensors and may not be used without authorization."
        />
        <P
          zh="数据集本身的知识产权由相应权利人享有，本协议不构成对数据集底层权利的转让。"
          en="Intellectual property in a Dataset itself belongs to its respective rights holder; these Terms do not transfer the underlying rights in any Dataset."
        />
      </LegalSection>

      <LegalSection n={11} zh="用户行为规范与禁止行为" en="User Conduct & Prohibited Activities">
        <P zh="您在使用本平台时不得从事下列行为：" en="When using the Platform, you must not:" />
        <List
          items={[
            ["上传或交易来源不合法、侵犯他人权利或违反法律法规的数据；", "upload or trade data that is unlawfully sourced, infringes others' rights, or violates laws and regulations;"],
            ["伪造来源声明、规避质检或实名认证；", "falsify Provenance Declarations, or circumvent quality inspection or real-name verification;"],
            ["利用本平台从事洗钱、欺诈、传播恶意程序或其他违法活动；", "use the Platform for money laundering, fraud, distribution of malware, or other unlawful activity;"],
            ["对平台系统进行未经授权的访问、抓取、干扰或攻击。", "engage in unauthorized access, scraping, interference, or attacks against Platform systems."],
          ]}
        />
      </LegalSection>

      <LegalSection n={12} zh="退款、纠纷与裁决" en="Refunds, Disputes & Adjudication">
        <P
          zh="买卖双方就交易发生争议的，可通过本平台发起纠纷处理。平台方可基于交易记录、交付指纹、质检与留痕信息进行调解或作出裁决，并据此指令存管资金的退款或结算。"
          en="In the event of a transaction dispute, either party may initiate dispute handling on the Platform. We may mediate or adjudicate based on transaction records, delivery fingerprints, inspection results, and audit logs, and instruct the custodian to refund or settle accordingly."
        />
        <P
          zh="平台裁决旨在高效解决交易争议，不影响当事人依法寻求其他救济的权利。"
          en="Platform adjudication aims to resolve transaction disputes efficiently and does not affect a party's right to seek other remedies under law."
        />
      </LegalSection>

      <LegalSection n={13} zh="免责声明与责任限制" en="Disclaimers & Limitation of Liability">
        <P
          zh="在法律允许的最大范围内，本平台按“现状”提供服务。平台方不对数据集的准确性、完整性、适用性或交易结果作出明示或默示担保。"
          en="To the maximum extent permitted by law, the Platform is provided “as is”. We make no express or implied warranties as to the accuracy, completeness, fitness, or outcome of any Dataset or transaction."
        />
        <P
          zh="在法律允许的范围内，平台方对用户的累计赔偿责任以引致责任的相关交易中用户实际支付给平台方的服务费总额为限；平台方不对间接、附带或惩罚性损失负责。本条不排除依法不可免除的责任。"
          en="To the extent permitted by law, our aggregate liability to a User is capped at the total Platform service fees actually paid by that User in the transaction giving rise to the liability; we are not liable for indirect, incidental, or punitive damages. This clause does not exclude liability that cannot be excluded under law."
        />
      </LegalSection>

      <LegalSection n={14} zh="违约处理与账户措施" en="Breach & Account Measures">
        <P
          zh="如您违反本协议或相关法律法规，平台方有权视情节采取警告、限制功能、下架数据集、冻结结算、暂停或终止账户等措施，并保留追究法律责任的权利。"
          en="If you breach these Terms or applicable laws, we may, depending on severity, issue warnings, restrict features, delist Datasets, freeze settlement, or suspend/terminate accounts, and reserve the right to pursue legal liability."
        />
      </LegalSection>

      <LegalSection n={15} zh="协议变更" en="Modifications">
        <P
          zh="平台方可根据法律法规或业务调整修订本协议，并通过平台公告或站内通知方式发布。变更生效后您继续使用本平台的，视为接受修订后的协议。"
          en="We may revise these Terms due to legal or business changes, published via Platform announcements or in-app notices. Continued use after the effective date of a revision constitutes acceptance."
        />
      </LegalSection>

      <LegalSection n={16} zh="适用法律与争议管辖" en="Governing Law & Jurisdiction">
        <P
          zh="本协议适用中华人民共和国法律（不含港澳台地区冲突法规则）。"
          en="These Terms are governed by the laws of the People's Republic of China (excluding the conflict-of-laws rules of the Hong Kong, Macau, and Taiwan regions)."
        />
        <P
          zh="因本协议产生的争议，双方友好协商解决；协商不成的，任一方均可向原告住所地人民法院提起诉讼，或提交杭州仲裁委员会仲裁。"
          en="Disputes arising from these Terms shall be resolved through friendly negotiation; failing which, either party may bring a lawsuit before the People's Court at the domicile of the plaintiff, or submit the dispute to the Hangzhou Arbitration Commission for arbitration."
        />
      </LegalSection>

      <LegalSection n={17} zh="其他" en="Miscellaneous">
        <P
          zh="本协议某一条款被认定为无效或不可执行的，不影响其余条款的效力。本协议的标题仅为方便阅读，不影响条款解释。"
          en="If any provision is held invalid or unenforceable, the remaining provisions remain in effect. Headings are for convenience only and do not affect interpretation."
        />
        <P
          zh={`如对本协议有任何疑问，请联系：${BRAND.contactEmail}`}
          en={`For questions about these Terms, contact: ${BRAND.contactEmail}`}
        />
      </LegalSection>

      <LegalFooterNav current="terms" />
    </article>
  );
}
