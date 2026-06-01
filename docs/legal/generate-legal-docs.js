const fs = require("fs");
const path = require("path");
const {
  Document, Packer, Paragraph, TextRun, HeadingLevel, AlignmentType,
  LevelFormat, BorderStyle, PageNumber, Header, Footer,
} = require("docx");

const BRAND = {
  name: "Verdant Oasis", nameZh: "绿洲",
  entity: "杭州科农绿洲生物科技有限公司",
  entityEn: "Hangzhou Kenong Oasis Biotechnology Co., Ltd.",
  uscc: "91330185MAE1A5P27M",
  addressZh: "浙江省杭州市临安区青山湖街道崇文路2088号8幢210室",
  addressEn: "Room 210, Building 8, No. 2088 Chongwen Road, Qingshanhu Street, Lin'an District, Hangzhou, Zhejiang Province, China",
  sloganEn: "In the data desert, we build a pure oasis.",
  sloganZh: "在数据荒漠中，我们筑起一片纯净绿洲。",
  philEn: "We build a pure oasis in the data desert — where clean, high-quality AI training data converges without pollution.",
  philZh: "我们在数据荒漠中筑起一片纯净绿洲——让高质量的 AI 训练数据在此汇聚，而非相互污染。",
  effective: "2026-06-01", updated: "2026-06-01",
  legalEmail: "legal@verdantoasis.cn", dpoEmail: "privacy@verdantoasis.cn",
  custodianZh: "资金存管机构【待实际支付机构名称确定后填写】",
  custodianEn: "the fund-custody institution [name to be filled in once the actual payment institution is confirmed]",
};

// ---- content model ----------------------------------------------------------
const P = (zh, en) => ({ t: "p", zh, en });
const L = (...items) => ({ t: "list", items }); // items: [zh, en]

const terms = {
  titleZh: "用户服务协议", titleEn: "Terms of Service",
  sections: [
    { n: 1, zh: "导言与协议接受", en: "Introduction & Acceptance", body: [
      P(`欢迎使用 Verdant Oasis（绿洲）AI 训练数据交易平台（下称"本平台"）。本平台由${BRAND.entity}（下称"我们"或"平台方"）运营。本《用户服务协议》（下称"本协议"）是您与平台方之间就使用本平台服务所订立的具有法律约束力的协议。`,
        `Welcome to Verdant Oasis, an AI training-data marketplace (the "Platform"), operated by ${BRAND.entityEn} ("we", "us", or the "Operator"). These Terms of Service (the "Terms") form a legally binding agreement between you and the Operator governing your use of the Platform.`),
      P(`您勾选同意、注册账户或以任何方式使用本平台，即表示您已阅读、理解并同意接受本协议全部条款。若您不同意，请勿使用本平台。`,
        `By checking the consent box, registering an account, or otherwise using the Platform, you acknowledge that you have read, understood, and agreed to be bound by all of these Terms. If you do not agree, do not use the Platform.`),
      P(`若您代表机构使用本平台，您声明并保证已获得该机构的充分授权，本协议对该机构具有约束力。`,
        `If you use the Platform on behalf of an organization, you represent and warrant that you are duly authorized to bind that organization, and these Terms bind that organization.`),
    ]},
    { n: 2, zh: "定义", en: "Definitions", body: [
      L(['"用户"：指注册并使用本平台的个人或机构，包括数据卖方与数据买方。', '"User": any individual or organization that registers and uses the Platform, including data Sellers and data Buyers.'],
        ['"数据集"：指卖方在本平台上架、用于人工智能训练等用途的数据产品。', '"Dataset": a data product listed by a Seller on the Platform for purposes such as AI training.'],
        ['"来源声明"：指卖方就数据集的合法来源、授权链条与合规状态所作的承诺与举证。', '"Provenance Declaration": the Seller\'s representations and supporting evidence regarding a Dataset\'s lawful origin, chain of authorization, and compliance status.'],
        [`"分账/存管"：指交易资金不进入平台自有账户，由${BRAND.custodianZh}按指令存管与结算的资金处理模式。`, `"Split Settlement / Custody": a fund-handling model in which transaction funds do not enter the Platform's own account but are held and settled by ${BRAND.custodianEn} per instruction.`]),
    ]},
    { n: 3, zh: "账户注册与实名认证", en: "Registration & Real-Name Verification", body: [
      P("本平台仅向中华人民共和国境内、已完成实名认证的用户开放。您应提供真实、准确、完整的注册与实名信息，并在信息变更时及时更新。",
        "The Platform is open only to real-name-verified Users within the mainland territory of the People's Republic of China. You must provide true, accurate, and complete registration and identity information and keep it current."),
      P("您应妥善保管账户凭证，对账户项下的全部行为负责。如发现账户被未经授权使用，应立即通知平台方。",
        "You are responsible for safeguarding your credentials and for all activity under your account. Notify us immediately of any unauthorized use."),
      P("平台方有权依法对用户身份进行核验。提供虚假信息或未通过实名认证的，平台方有权拒绝、限制或终止提供服务。",
        "We may verify User identity as required by law. We may refuse, restrict, or terminate service for false information or failure to pass real-name verification."),
    ]},
    { n: 4, zh: "平台角色与服务性质", en: "Platform Role & Nature of Service", body: [
      P("本平台是为买卖双方提供信息撮合、质量检测、合约签订与资金存管支持的技术服务平台。除另有明确约定外，平台方不是数据交易的买方或卖方，亦不对数据集的内容、质量、权属作出担保。",
        "The Platform is a technology service that provides matchmaking, quality inspection, contracting, and fund-custody support between Buyers and Sellers. Unless expressly stated otherwise, the Operator is not a party to any Dataset transaction and does not warrant the content, quality, or title of any Dataset."),
      P("数据交易的权利义务由买卖双方通过本平台签订的交易合约确定。平台方提供的质检、形式审查等措施旨在提升交易信任，不构成对合规性或适用性的保证。",
        "The rights and obligations of a Dataset transaction are governed by the transaction contract entered into by Buyer and Seller via the Platform. Quality inspection and formal review provided by us aim to enhance trust and do not constitute a guarantee of compliance or fitness for purpose."),
    ]},
    { n: 5, zh: "数据上架与来源合规", en: "Dataset Listing & Provenance Compliance", body: [
      P("卖方上架数据集前，必须就数据来源作出真实、完整的来源声明，并签署相关合规承诺。卖方应保证：",
        "Before listing a Dataset, the Seller must make a true and complete Provenance Declaration and sign the related compliance undertakings. The Seller warrants that:"),
      L(["对数据集享有合法权利或已获得充分授权，可在本平台流通与许可使用；", "it holds lawful rights to, or sufficient authorization for, the Dataset to be circulated and licensed on the Platform;"],
        ["数据的收集、处理符合《个人信息保护法》《数据安全法》《网络安全法》等法律法规；", "the collection and processing of the data complies with the Personal Information Protection Law (PIPL), Data Security Law, Cybersecurity Law, and other applicable laws;"],
        ["数据集中如包含个人信息，已取得必要的同意或具备其他合法性基础，并已做必要的去标识化/匿名化处理；", "where the Dataset contains personal information, the necessary consent or other lawful basis has been obtained, and necessary de-identification/anonymization has been performed;"],
        ["数据不含国家秘密、未经授权的商业秘密、违法或侵权内容。", "the data contains no state secrets, unauthorized trade secrets, or unlawful or infringing content."]),
      P("平台方对来源声明进行形式审查并全程留痕，但形式审查不免除卖方的合规与担保责任。因卖方来源不实导致的责任由卖方承担。",
        "We conduct a formal (non-substantive) review of Provenance Declarations with full audit logging. Such review does not relieve the Seller of its compliance and warranty obligations. Liability arising from inaccurate provenance rests with the Seller."),
    ]},
    { n: "5.1", zh: "AI 训练数据特别声明", en: "Special Statement on AI Training Data", body: [
      P("用户理解并同意：本平台上架或交易的数据集可能被用于人工智能模型训练。卖方承诺并保证其数据集不包含国家秘密、未经充分去标识化的个人敏感信息、或可能引发算法偏见、歧视、伦理风险的内容。平台方不对买方使用数据集所产生的任何下游法律风险、伦理风险或模型训练结果承担责任。",
        "Users understand and agree that Datasets listed or traded on the Platform may be used for training artificial-intelligence models. The Seller undertakes and warrants that its Dataset contains no state secrets, no sensitive personal information that has not been adequately de-identified, and no content that may give rise to algorithmic bias, discrimination, or ethical risk. The Operator bears no liability for any downstream legal risk, ethical risk, or model-training outcome arising from the Buyer's use of a Dataset."),
    ]},
    { n: 6, zh: "质量检测", en: "Quality Inspection", body: [
      P("数据集上传后将进入异步质检流程，包括格式校验、统计分析、去重检测与个人信息（PII）扫描。质检结果作为买方决策参考。",
        "Uploaded Datasets undergo asynchronous quality inspection, including format validation, statistical analysis, de-duplication, and personal-information (PII) scanning. Inspection results serve as a reference for Buyers."),
      P("质检为自动化辅助手段，可能存在局限。买方应自行评估数据集是否满足其用途。",
        "Inspection is an automated aid with inherent limitations. Buyers should independently assess whether a Dataset meets their intended use."),
    ]},
    { n: 7, zh: "交易、支付与资金存管", en: "Transactions, Payment & Fund Custody", body: [
      P(`买方下单后，订单进入严格状态机管理（创建、已支付、已交付、已确认、已结算等）。交易资金不进入平台自有账户，由${BRAND.custodianZh}按本协议与交易指令进行存管与分账结算。`,
        `Once a Buyer places an order, it is managed by a strict state machine (created, paid, delivered, confirmed, settled, etc.). Transaction funds do not enter the Platform's own account; ${BRAND.custodianEn} holds and settles them via split settlement per these Terms and the transaction instructions.`),
      P("买方确认收货后，资金按约定结算给卖方（扣除平台服务费）。平台服务费的费率与计费方式以下单时页面展示为准。",
        "Upon the Buyer's confirmation of receipt, funds are settled to the Seller per agreement (net of Platform service fees). Fee rates and methods are as displayed at the time of ordering."),
    ]},
    { n: 8, zh: "交付、许可与使用限制", en: "Delivery, License & Usage Restrictions", body: [
      P("数据集通过临时下载链接交付，并施加交付指纹以支持溯源。买方获得的是交易合约约定范围内的使用许可，而非数据的所有权。",
        "Datasets are delivered via temporary download links with delivery fingerprints to support traceability. The Buyer obtains a license within the scope of the transaction contract, not ownership of the data."),
      P("除许可范围明确允许外，买方不得转售、再许可、公开传播或超范围使用所购数据集。",
        "Except as expressly permitted by the license scope, the Buyer may not resell, sub-license, publicly distribute, or use the purchased Dataset beyond the granted scope."),
    ]},
    { n: 9, zh: "知识产权", en: "Intellectual Property", body: [
      P('本平台的软件、界面、商标（包括 "Verdant Oasis / 绿洲" 标识）、Slogan 及相关内容的知识产权归平台方或权利人所有，未经授权不得使用。',
        'Intellectual property in the Platform\'s software, interface, trademarks (including the "Verdant Oasis" mark), Slogan, and related content belongs to the Operator or its licensors and may not be used without authorization.'),
      P("数据集本身的知识产权由相应权利人享有，本协议不构成对数据集底层权利的转让。",
        "Intellectual property in a Dataset itself belongs to its respective rights holder; these Terms do not transfer the underlying rights in any Dataset."),
    ]},
    { n: 10, zh: "用户行为规范与禁止行为", en: "User Conduct & Prohibited Activities", body: [
      P("您在使用本平台时不得从事下列行为：", "When using the Platform, you must not:"),
      L(["上传或交易来源不合法、侵犯他人权利或违反法律法规的数据；", "upload or trade data that is unlawfully sourced, infringes others' rights, or violates laws and regulations;"],
        ["伪造来源声明、规避质检或实名认证；", "falsify Provenance Declarations, or circumvent quality inspection or real-name verification;"],
        ["利用本平台从事洗钱、欺诈、传播恶意程序或其他违法活动；", "use the Platform for money laundering, fraud, distribution of malware, or other unlawful activity;"],
        ["对平台系统进行未经授权的访问、抓取、干扰或攻击。", "engage in unauthorized access, scraping, interference, or attacks against Platform systems."]),
    ]},
    { n: 11, zh: "退款、纠纷与裁决", en: "Refunds, Disputes & Adjudication", body: [
      P("买卖双方就交易发生争议的，可通过本平台发起纠纷处理。平台方可基于交易记录、交付指纹、质检与留痕信息进行调解或作出裁决，并据此指令存管资金的退款或结算。",
        "In the event of a transaction dispute, either party may initiate dispute handling on the Platform. We may mediate or adjudicate based on transaction records, delivery fingerprints, inspection results, and audit logs, and instruct the custodian to refund or settle accordingly."),
      P("平台裁决旨在高效解决交易争议，不影响当事人依法寻求其他救济的权利。",
        "Platform adjudication aims to resolve transaction disputes efficiently and does not affect a party's right to seek other remedies under law."),
    ]},
    { n: 12, zh: "免责声明与责任限制", en: "Disclaimers & Limitation of Liability", body: [
      P('在法律允许的最大范围内，本平台按"现状"提供服务。平台方不对数据集的准确性、完整性、适用性或交易结果作出明示或默示担保。',
        'To the maximum extent permitted by law, the Platform is provided "as is". We make no express or implied warranties as to the accuracy, completeness, fitness, or outcome of any Dataset or transaction.'),
      P("在法律允许的范围内，平台方对用户的累计赔偿责任以引致责任的相关交易中用户实际支付给平台方的服务费总额为限；平台方不对间接、附带或惩罚性损失负责。本条不排除依法不可免除的责任。",
        "To the extent permitted by law, our aggregate liability to a User is capped at the total Platform service fees actually paid by that User in the transaction giving rise to the liability; we are not liable for indirect, incidental, or punitive damages. This clause does not exclude liability that cannot be excluded under law."),
    ]},
    { n: 13, zh: "违约处理与账户措施", en: "Breach & Account Measures", body: [
      P("如您违反本协议或相关法律法规，平台方有权视情节采取警告、限制功能、下架数据集、冻结结算、暂停或终止账户等措施，并保留追究法律责任的权利。",
        "If you breach these Terms or applicable laws, we may, depending on severity, issue warnings, restrict features, delist Datasets, freeze settlement, or suspend/terminate accounts, and reserve the right to pursue legal liability."),
    ]},
    { n: 14, zh: "协议变更", en: "Modifications", body: [
      P("平台方可根据法律法规或业务调整修订本协议，并通过平台公告或站内通知方式发布。变更生效后您继续使用本平台的，视为接受修订后的协议。",
        "We may revise these Terms due to legal or business changes, published via Platform announcements or in-app notices. Continued use after the effective date of a revision constitutes acceptance."),
    ]},
    { n: 15, zh: "适用法律与争议管辖", en: "Governing Law & Jurisdiction", body: [
      P("本协议适用中华人民共和国法律（不含港澳台地区冲突法规则）。",
        "These Terms are governed by the laws of the People's Republic of China (excluding the conflict-of-laws rules of the Hong Kong, Macau, and Taiwan regions)."),
      P("因本协议产生的争议，双方友好协商解决；协商不成的，任一方均可向原告住所地人民法院提起诉讼，或提交杭州仲裁委员会仲裁。",
        "Disputes arising from these Terms shall be resolved through friendly negotiation; failing which, either party may bring a lawsuit before the People's Court at the domicile of the plaintiff, or submit the dispute to the Hangzhou Arbitration Commission for arbitration."),
    ]},
    { n: 16, zh: "其他", en: "Miscellaneous", body: [
      P("本协议某一条款被认定为无效或不可执行的，不影响其余条款的效力。本协议的标题仅为方便阅读，不影响条款解释。",
        "If any provision is held invalid or unenforceable, the remaining provisions remain in effect. Headings are for convenience only and do not affect interpretation."),
      P(`如对本协议有任何疑问，请联系：${BRAND.legalEmail}`, `For questions about these Terms, contact: ${BRAND.legalEmail}`),
    ]},
  ],
};

const privacy = {
  titleZh: "隐私政策", titleEn: "Privacy Policy",
  sections: [
    { n: 1, zh: "引言", en: "Introduction", body: [
      P(`${BRAND.entity}（下称"我们"）深知个人信息对您的重要性。本《隐私政策》说明我们在您使用 Verdant Oasis（绿洲）平台时如何收集、使用、存储、共享与保护您的个人信息，以及您所享有的权利。`,
        `${BRAND.entityEn} ("we", "us") understands the importance of your personal information. This Privacy Policy explains how we collect, use, store, share, and protect your personal information when you use the Verdant Oasis platform, and the rights available to you.`),
      P("我们依据《个人信息保护法》《数据安全法》《网络安全法》等法律法规处理个人信息。请您仔细阅读本政策；勾选同意或使用本平台，即表示您理解并同意本政策。",
        "We process personal information in accordance with the PIPL, Data Security Law, Cybersecurity Law, and other applicable laws. Please read this Policy carefully; by consenting or using the Platform, you acknowledge and agree to it."),
    ]},
    { n: 2, zh: "我们收集的个人信息", en: "Personal Information We Collect", body: [
      P("我们仅在为实现具体功能所必需的范围内收集个人信息：", "We collect personal information only to the extent necessary to deliver specific functions:"),
      L(["注册信息：手机号/邮箱、账户密码（加密存储）。", "Registration: mobile number/email, account password (stored encrypted)."],
        ["实名认证信息：姓名、证件号码、机构信息等，用于身份核验与合规准入。", "Real-name verification: name, ID number, organization details — for identity verification and compliant onboarding."],
        ["交易信息：订单记录、支付与结算信息、上架/购买的数据集记录。", "Transaction: order records, payment and settlement information, listing/purchase records."],
        ["设备与日志信息：IP 地址、设备标识、浏览器类型、访问日志，用于安全风控与服务运行。", "Device & logs: IP address, device identifiers, browser type, access logs — for security/risk control and service operation."]),
      P("我们不会主动收集与服务无关的敏感个人信息。如某项功能需处理敏感个人信息，我们将单独征得您的同意。",
        "We do not proactively collect sensitive personal information unrelated to the service. Where a function requires processing sensitive personal information, we will obtain your separate consent."),
    ]},
    { n: 3, zh: "处理目的与合法性基础", en: "Purposes & Lawful Basis", body: [
      P("我们基于以下合法性基础处理您的个人信息（《个人信息保护法》第十三条）：", "We process your personal information on the following lawful bases (PIPL Article 13):"),
      L(["为订立和履行您作为一方当事人的合约（如交易、交付、结算）所必需；", "necessary to conclude or perform a contract to which you are a party (e.g., transaction, delivery, settlement);"],
        ["为履行法定义务（如实名制、反洗钱、配合监管）所必需；", "necessary to perform statutory obligations (e.g., real-name requirements, anti-money-laundering, regulatory cooperation);"],
        ["在征得您同意的范围内，用于提升服务与安全保障；", "within the scope of your consent, to improve services and security;"],
        ["法律法规规定的其他情形。", "other circumstances provided by laws and regulations."]),
    ]},
    { n: 4, zh: "Cookie 与同类技术", en: "Cookies & Similar Technologies", body: [
      P("我们使用 Cookie 及本地存储等技术维持登录态、保障安全并优化体验。您可通过浏览器设置管理或清除 Cookie，但这可能影响部分功能的使用。",
        "We use Cookies and local storage to maintain login state, ensure security, and optimize experience. You may manage or clear Cookies via browser settings, which may affect some functionality."),
    ]},
    { n: 5, zh: "个人信息的对外提供与委托处理", en: "Sharing & Entrusted Processing", body: [
      P("为实现平台功能，我们可能在最小必要范围内向下列第三方提供或委托处理个人信息，并以协议约束其合规处理：",
        "To deliver Platform functions, we may share with, or entrust processing to, the following third parties on a minimum-necessary basis, bound by agreements to ensure compliant processing:"),
      L([`${BRAND.custodianZh}：用于支付、分账与结算。`, "Fund-custody institution [name to be filled in once the actual payment institution is confirmed]: for payment, split settlement, and settlement."],
        ["实名核验服务商：用于身份认证。", "Identity-verification providers: for real-name authentication."],
        ["云存储与基础设施服务商：用于数据集与系统数据的存储与运行。", "Cloud storage and infrastructure providers: for storing and operating Dataset and system data."],
        ["质量检测相关服务（如适用）：用于数据集质检。", "Quality-inspection-related services (if applicable): for Dataset inspection."],
        ["依法配合的司法或监管机关。", "Judicial or regulatory authorities, as required by law."]),
      P("除上述情形、获得您单独同意或法律法规另有规定外，我们不会向第三方提供您的个人信息。",
        "Except as above, with your separate consent, or as otherwise required by law, we will not provide your personal information to third parties."),
    ]},
    { n: 6, zh: "个人信息的存储", en: "Storage of Personal Information", body: [
      P("您的个人信息存储于中华人民共和国境内。我们仅在实现处理目的所必需的最短期限内保存您的个人信息，法律法规另有规定的从其规定；超出保存期限的，我们将删除或匿名化处理。",
        "Your personal information is stored within the territory of the People's Republic of China. We retain it only for the shortest period necessary to achieve the processing purpose, unless laws require otherwise; beyond the retention period, we delete or anonymize it."),
      P("本平台原则上不向境外提供个人信息。如确需跨境提供，我们将依《个人信息保护法》履行单独同意、个人信息保护影响评估及法定路径（如安全评估、标准合同或认证）等要求。",
        "As a rule, the Platform does not provide personal information overseas. Where cross-border transfer is genuinely necessary, we will fulfill PIPL requirements including separate consent, a personal-information protection impact assessment, and a statutory pathway (e.g., security assessment, standard contract, or certification)."),
    ]},
    { n: 7, zh: "个人信息安全", en: "Information Security", body: [
      P("我们采取符合行业标准的安全措施保护个人信息，包括传输与存储加密、访问控制、最小权限、操作留痕与安全审计。但请注意，互联网环境并非绝对安全。",
        "We adopt industry-standard security measures, including encryption in transit and at rest, access control, least-privilege, audit logging, and security audits. Please note that no internet environment is absolutely secure."),
      P("如发生个人信息安全事件，我们将依法采取补救措施，并按规定向您和监管部门告知。",
        "In the event of a personal-information security incident, we will take remedial measures and notify you and the authorities as required by law."),
    ]},
    { n: 8, zh: "您的权利", en: "Your Rights", body: [
      P("在法律法规规定的范围内，您对自己的个人信息享有以下权利：", "Within the scope provided by law, you have the following rights over your personal information:"),
      L(["查阅、复制您的个人信息；", "access and copy your personal information;"],
        ["更正、补充不准确或不完整的个人信息；", "correct or supplement inaccurate or incomplete information;"],
        ["删除符合法定情形的个人信息；", "delete personal information where statutory conditions are met;"],
        ["撤回您此前作出的同意；", "withdraw consent previously given;"],
        ["注销账户；", "deregister your account;"],
        ["在符合条件时请求将个人信息转移至您指定的处理者。", "request transfer of your personal information to a designated handler where conditions are met."]),
      P(`您可通过 ${BRAND.dpoEmail} 行使上述权利。我们将在法定期限内核验并响应您的请求。`,
        `You may exercise these rights via ${BRAND.dpoEmail}. We will verify and respond within the statutory time limit.`),
    ]},
    { n: 9, zh: "未成年人保护", en: "Protection of Minors", body: [
      P("本平台面向具备完全民事行为能力的用户，不面向未成年人。我们不会在明知的情况下收集未成年人的个人信息；如发现，将依法及时删除。",
        "The Platform is intended for users with full civil capacity and is not directed at minors. We do not knowingly collect minors' personal information; if discovered, we will delete it promptly as required by law."),
    ]},
    { n: 10, zh: "第三方链接与服务", en: "Third-Party Links & Services", body: [
      P("本平台可能包含第三方链接或服务，其隐私实践由该第三方负责，本政策不适用于第三方。建议您在使用前查阅其隐私政策。",
        "The Platform may contain third-party links or services whose privacy practices are their own responsibility; this Policy does not apply to them. We recommend reviewing their privacy policies before use."),
    ]},
    { n: 11, zh: "本政策的更新", en: "Updates to this Policy", body: [
      P("我们可能适时更新本政策。对于重大变更，我们将以显著方式（如公告或站内通知）告知。更新后您继续使用本平台的，视为接受更新后的政策。",
        "We may update this Policy from time to time. For material changes, we will notify you prominently (e.g., announcement or in-app notice). Continued use after an update constitutes acceptance."),
    ]},
    { n: 12, zh: "个人信息保护负责人与联系方式", en: "Contact & Person in Charge of PI Protection", body: [
      P(`如您对本政策或个人信息处理有任何疑问、意见或投诉，可联系我们的个人信息保护负责人：${BRAND.dpoEmail}。运营主体：${BRAND.entity}。`,
        `For any questions, comments, or complaints regarding this Policy or our processing of personal information, contact our person in charge of personal-information protection: ${BRAND.dpoEmail}. Operator: ${BRAND.entityEn}.`),
      P("如您认为我们的处理损害了您的合法权益，且与我们协商未果，您有权向网信、市场监管等有权部门投诉或举报。",
        "If you believe our processing harms your lawful rights and we fail to resolve it, you may complain or report to competent authorities such as the cyberspace administration or market regulator."),
    ]},
  ],
};

const checklist = [
  ["经营范围（已确认覆盖）", "经核对营业执照，一般项目已包含“数据处理服务、数据处理和存储支持服务、人工智能公共数据平台、人工智能行业应用系统集成服务、人工智能基础软件开发、人工智能应用软件开发、信息系统集成服务、技术开发、技术服务、软件开发”等，许可项目包含“互联网信息服务、第一类/第二类增值电信业务”，已覆盖本平台 AI 训练数据交易业务，原“经营范围不符”风险已消除。"],
  ["增值电信/ICP 许可与备案（待办）", "上述“互联网信息服务、第一类/第二类增值电信业务”属营业执照中的许可项目，经营范围虽已列明，仍须在平台正式上线前实际取得相应 ICP 经营许可证及 EDI（在线数据处理与交易处理业务）等增值电信业务经营许可，并完成 ICP 备案后方可开展。"],
  ["资金存管机构名称", `已统一占位为“${BRAND.custodianZh}”，需填入实际合作方（与代码中 Stripe Connect / 微信支付宝分账方一致）。`],
  ["管辖与争议解决（已定稿）", "已按律师意见定稿：适用中华人民共和国法律（不含港澳台地区冲突法规则）；争议经协商不成的，可向原告住所地人民法院提起诉讼，或提交杭州仲裁委员会仲裁。如需进一步指定具体受理法院，请确认。"],
  ["联系邮箱（已更新）", `已更新为公司域名邮箱 ${BRAND.legalEmail} / ${BRAND.dpoEmail}，请在企业邮箱实际开通后启用，并停用个人邮箱。`],
  ["跨境条款", "现按“原则上不出境”起草；若实际有境外买家或海外云，需要展开 PIPL 第三章跨境路径。"],
];

// ---- DOCX builder -----------------------------------------------------------
const ZH = (t, o = {}) => new TextRun({ text: t, font: "PingFang SC", ...o });
const EN = (t, o = {}) => new TextRun({ text: t, font: "Arial", color: "595959", italics: true, ...o });

function brandBlock() {
  const out = [];
  out.push(new Paragraph({ spacing: { after: 60 }, children: [new TextRun({ text: `${BRAND.name} · ${BRAND.nameZh}`, bold: true, color: "1F7A33" })] }));
  out.push(new Paragraph({ spacing: { after: 20 }, children: [new TextRun({ text: BRAND.sloganEn, bold: true, italics: true, color: "1F7A33" })] }));
  out.push(new Paragraph({ spacing: { after: 80 }, children: [ZH(BRAND.sloganZh, { bold: true, color: "1F7A33" })] }));
  out.push(new Paragraph({ spacing: { after: 20 }, children: [new TextRun({ text: BRAND.philEn, color: "1F7A33" })] }));
  out.push(new Paragraph({ spacing: { after: 160 }, border: { bottom: { style: BorderStyle.SINGLE, size: 6, color: "C6E5CC", space: 6 } }, children: [ZH(BRAND.philZh, { color: "1F7A33" })] }));
  return out;
}

function docHeader(doc) {
  const out = [];
  out.push(new Paragraph({ heading: HeadingLevel.HEADING_1, children: [new TextRun(`${doc.titleZh}  /  ${doc.titleEn}`)] }));
  out.push(...brandBlock());
  out.push(new Paragraph({ spacing: { after: 20 }, children: [ZH(`运营主体 Operator：${BRAND.entity}（`), new TextRun({ text: BRAND.entityEn, italics: false }), ZH("）")] }));
  out.push(new Paragraph({ spacing: { after: 20 }, children: [ZH(`统一社会信用代码 USCC：${BRAND.uscc}`)] }));
  out.push(new Paragraph({ spacing: { after: 20 }, children: [ZH(`注册地址 Registered address：${BRAND.addressZh}`)] }));
  out.push(new Paragraph({ spacing: { after: 160 }, children: [ZH(`生效日期 Effective：${BRAND.effective}　|　更新日期 Updated：${BRAND.updated}`)] }));
  out.push(new Paragraph({ spacing: { after: 240 }, shading: { fill: "FFF7E6" }, border: { top: { style: BorderStyle.SINGLE, size: 4, color: "F0C36D" }, bottom: { style: BorderStyle.SINGLE, size: 4, color: "F0C36D" }, left: { style: BorderStyle.SINGLE, size: 4, color: "F0C36D" }, right: { style: BorderStyle.SINGLE, size: 4, color: "F0C36D" } },
    children: [ZH("草稿提示：本文本为待律师审核的草稿版本，部分条款（如管辖法院、存管机构名称、跨境安排）需结合最终商业与合规安排定稿。", { color: "8A6D3B", size: 18 })] }));
  return out;
}

function renderBody(item) {
  if (item.t === "p") {
    return [
      new Paragraph({ spacing: { after: 20 }, children: [ZH(item.zh)] }),
      new Paragraph({ spacing: { after: 160 }, children: [EN(item.en)] }),
    ];
  }
  // list
  const out = [];
  for (const [zh, en] of item.items) {
    out.push(new Paragraph({ numbering: { reference: "bul", level: 0 }, spacing: { after: 20 }, children: [ZH(zh)] }));
    out.push(new Paragraph({ numbering: { reference: "bul", level: 0 }, spacing: { after: 120 }, children: [EN(en)] }));
  }
  return out;
}

function renderDoc(doc) {
  const out = [...docHeader(doc)];
  for (const s of doc.sections) {
    out.push(new Paragraph({ heading: HeadingLevel.HEADING_2, children: [new TextRun(`${s.n}. ${s.zh}  /  ${s.en}`)] }));
    for (const b of s.body) out.push(...renderBody(b));
  }
  return out;
}

function renderChecklist() {
  const out = [];
  out.push(new Paragraph({ heading: HeadingLevel.HEADING_1, children: [new TextRun("附：待律师/业务确认清单 / Open Points for Counsel")] }));
  out.push(new Paragraph({ spacing: { after: 160 }, children: [ZH("以下占位或待定事项需在定稿前确认：", { size: 20 })] }));
  checklist.forEach(([k, v], i) => {
    out.push(new Paragraph({ numbering: { reference: "num", level: 0 }, spacing: { after: 120 }, children: [ZH(`${k}　`, { bold: true }), ZH(v)] }));
  });
  return out;
}

const doc = new Document({
  styles: {
    default: { document: { run: { font: "Arial", size: 21 } } },
    paragraphStyles: [
      { id: "Heading1", name: "Heading 1", basedOn: "Normal", next: "Normal", quickFormat: true, run: { size: 30, bold: true, font: "Arial", color: "14532D" }, paragraph: { spacing: { before: 360, after: 160 }, outlineLevel: 0 } },
      { id: "Heading2", name: "Heading 2", basedOn: "Normal", next: "Normal", quickFormat: true, run: { size: 24, bold: true, font: "Arial", color: "1F2937" }, paragraph: { spacing: { before: 240, after: 100 }, outlineLevel: 1 } },
    ],
  },
  numbering: { config: [
    { reference: "bul", levels: [{ level: 0, format: LevelFormat.BULLET, text: "•", alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 540, hanging: 280 } } } }] },
    { reference: "num", levels: [{ level: 0, format: LevelFormat.DECIMAL, text: "%1.", alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 540, hanging: 280 } } } }] },
  ]},
  sections: [{
    properties: { page: { size: { width: 11906, height: 16838 }, margin: { top: 1440, right: 1440, bottom: 1440, left: 1440 } } },
    footers: { default: new Footer({ children: [new Paragraph({ alignment: AlignmentType.CENTER, children: [new TextRun({ text: `${BRAND.name} · ${BRAND.sloganZh}　—　`, size: 16, color: "999999" }), new TextRun({ text: "第 ", size: 16, color: "999999" }), new TextRun({ children: [PageNumber.CURRENT], size: 16, color: "999999" }), new TextRun({ text: " 页", size: 16, color: "999999" })] })] }) },
    children: [
      ...renderDoc(terms),
      new Paragraph({ pageBreakBefore: true, children: [] }),
      ...renderDoc(privacy),
      new Paragraph({ pageBreakBefore: true, children: [] }),
      ...renderChecklist(),
    ],
  }],
});

// ---- Markdown builder -------------------------------------------------------
function mdQuote(s) { return s.split("\n").map((l) => "> " + l).join("\n"); }
function mdDoc(doc) {
  let m = `# ${doc.titleZh} / ${doc.titleEn}\n\n`;
  m += `> **${BRAND.name} · ${BRAND.nameZh}**  \n> *${BRAND.sloganEn}*  \n> *${BRAND.sloganZh}*  \n>\n> ${BRAND.philEn}  \n> ${BRAND.philZh}\n\n`;
  m += `**运营主体 Operator：** ${BRAND.entity}（${BRAND.entityEn}）  \n`;
  m += `**统一社会信用代码 USCC：** ${BRAND.uscc}  \n`;
  m += `**注册地址 Registered address：** ${BRAND.addressZh}  \n`;
  m += `**生效日期 Effective：** ${BRAND.effective} ｜ **更新日期 Updated：** ${BRAND.updated}\n\n`;
  m += `> ⚠️ **草稿提示 / Draft notice：** 本文本为待律师审核的草稿版本，部分条款（如管辖法院、存管机构名称、跨境安排）需结合最终商业与合规安排定稿。\n\n`;
  for (const s of doc.sections) {
    m += `### ${s.n}. ${s.zh} / ${s.en}\n`;
    for (const b of s.body) {
      if (b.t === "p") m += `${b.zh}  \n*${b.en}*\n\n`;
      else { for (const [zh, en] of b.items) m += `- ${zh} *${en}*\n`; m += "\n"; }
    }
  }
  return m;
}
let md = mdDoc(terms) + "\n---\n\n" + mdDoc(privacy);
md += "\n---\n\n## 附：待律师/业务确认清单 / Open Points for Counsel\n\n以下占位或待定事项需在定稿前确认：\n\n";
checklist.forEach(([k, v], i) => { md += `${i + 1}. **${k}** — ${v}\n`; });

const outDir = path.resolve(__dirname);
fs.writeFileSync(path.join(outDir, "verdant-oasis-legal-zh-en.md"), md, "utf8");

Packer.toBuffer(doc).then((buf) => {
  fs.writeFileSync(path.join(outDir, "Verdant-Oasis-Terms-and-Privacy-zh-en.docx"), buf);
  console.log("WROTE md + docx to", outDir);
});
