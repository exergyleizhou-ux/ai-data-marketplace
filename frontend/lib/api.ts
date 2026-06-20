// Typed client for the marketplace backend. Handles the uniform response
// envelope { code, message, data, request_id }, Bearer auth, and one automatic
// access-token refresh on 401.

const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api/v1";
// Origin without the /api/v1 suffix — for absolute links the API returns (e.g. download_url).
export const API_ORIGIN = BASE.replace(/\/api\/v1\/?$/, "");

const ACCESS_KEY = "adm_access";
const REFRESH_KEY = "adm_refresh";

export type Tokens = { access_token: string; refresh_token: string; expires_in: number };
export type User = {
  id: string;
  account: string;
  account_type: string;
  role: string;
  kyc_status: string;
  status: string;
  totp_enabled?: boolean;
};
export type AuthResult = { user: User; tokens: Tokens };

export type LoginResult = {
  user?: User;
  tokens?: Tokens;
  need_2fa?: boolean;
  challenge_token?: string;
};

export type Enroll2FAResult = {
  otpauth_url: string;
  secret: string;
  recovery_codes: string[];
};

export type SourceDeclaration = {
  source: string;
  collection_method: string;
  contains_pii: boolean;
  license_scope: string;
  commitment: boolean;
};
export type ComputeSignal = {
  dataset_id: string;
  enabled: boolean;
  trust_level: string;
  allow_federated: boolean;
  allow_psi: boolean;
  jobs_run: number;
};

export type Dataset = {
  id: string;
  seller_id: string;
  title: string;
  description: string;
  data_type: string;
  domain?: string;
  license_type: string;
  suggested_price_cents?: number;
  final_price_cents?: number;
  status: string;
  total_size_bytes: number;
  sample_count: number;
  source_declaration?: SourceDeclaration;
  source_signed_at?: string;
  current_version_id?: string;
  created_at?: string;
  // Browse-time quality signal (present on catalog listings).
  quality_verified?: boolean;
  authenticity_band?: "clean" | "review" | "suspect" | string;
  authenticity_score?: number;
  datasheet?: Datasheet;
};

export type Datasheet = {
  intended_uses?: string;
  out_of_scope_uses?: string;
  composition?: string;
  collection_process?: string;
  preprocessing?: string;
  limitations?: string;
  ethical_considerations?: string;
  update_policy?: string;
  languages?: string[];
};
export type Order = {
  id: string;
  buyer_id: string;
  seller_id: string;
  dataset_id: string;
  license_type: string;
  amount_cents: number;
  platform_fee_cents: number;
  seller_amount_cents: number;
  status: string;
  product_type?: string; // download | compute
  created_at?: string;
};
export type Earnings = {
  settled_cents: number;
  pending_cents: number;
  withdrawable_cents: number;
  settled_orders: number;
  pending_orders: number;
};
export type Review = {
  id: string;
  dataset_id: string;
  score: number;
  comment?: string;
  issue_flag: boolean;
  created_at?: string;
};
export type KYC = {
  id: string;
  user_id?: string;
  type: string;
  real_name?: string;
  company_name?: string;
  verify_status: string;
  material_urls?: string[];
  created_at?: string;
};
export type Preview = {
  lines: string[];
  line_count: number;
  dataset_sample_count: number;
  truncated: boolean;
};

export type QualityCheck = {
  type: "format" | "stats" | "dedup" | "pii" | "pii_redaction" | "authenticity" | "schema" | string;
  result: "pass" | "warn" | "fail";
  report: Record<string, unknown>;
  created_at?: string;
};

export type VersionInfo = { version_no: number; changelog?: string; created_at: string };

export type Certificate = {
  status: "registered" | "pending" | string;
  certificate_id?: string;
  content_sha256?: string;
  version_no?: number;
  registered_at?: string;
  operator?: string;
  quality?: Record<string, string>;
  statement_zh?: string;
  statement_en?: string;
};

export class ApiError extends Error {
  code: number;
  status: number;
  constructor(code: number, status: number, message: string) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

// --- token storage ---
// --- compute-to-data (C2D / 可用不可见) ---
export type ComputeOffer = {
  dataset_id: string;
  enabled: boolean;
  allow_custom: boolean;
  allowed_algorithm_ids: string[];
  price_cents: number;
  max_runtime_secs: number;
  max_output_bytes: number;
  max_output_files: number;
  dp_epsilon?: number | null;
  dp_epsilon_total?: number | null;
  return_logs: boolean;
  review_output: boolean;
  trust_level: string; // L1 | L2 | L3
  allow_federated?: boolean; // P4-a: opt dataset into federated (L3) use
  allow_psi?: boolean; // Direction D: opt dataset into PSI (distinct consent from federated)
};
export type ComputeAlgorithm = {
  id: string;
  name: string;
  runtime: string;
  output_kind: string; // model | metrics | table | aggregate
  trusted: boolean;
  status: string;
  version: number;
  params_schema?: Record<string, unknown>;
};
export type ComputeEntitlement = {
  id: string;
  dataset_id: string;
  buyer_id: string;
  jobs_quota: number;
  jobs_used: number;
  status: string;
};
export type ComputeJob = {
  id: string;
  dataset_id: string;
  status: string; // created|queued|running|output_pending|output_reviewing|released|failed|rejected|canceled
  algorithm_id?: string;
  output_kind?: string;
  output_bytes?: number;
  error?: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
};
// Federated (L3) job: fans out one sandbox sub-job per dataset, aggregates the
// local model params with FedAvg into a joint model. Raw data never leaves a
// sandbox; only the joint model is buyer-visible.
export type FederatedJob = {
  id: string;
  buyer_id: string;
  algorithm_id?: string;
  dataset_ids: string[];
  mode: string;
  status: string; // created|fanout|aggregating|released|failed|rejected
  min_participants: number;
  dp_epsilon?: number | null;
  output_bytes?: number;
  failure_code?: string;
  created_at?: string;
};

// L2 remote-attestation report (design P3).
export type ComputeAttestation = {
  format: string;
  measurement: string; // algorithm image digest the enclave ran
  job_id: string;
  output_sha: string;
  signer: string; // mock-tee | tdx | sev-snp | sgx-dcap
  verified?: boolean;
};

export type OutboxEntry = {
  order_id: string;
  status: string;
  attempts: number;
  last_error?: string | null;
  next_attempt_at: string;
  created_at: string;
  updated_at: string;
};

export type ReconciliationPoint = {
  date: string;
  gmv_cents: number;
  settled_gmv_cents: number;
  platform_fees_cents: number;
  orders: number;
  settled_orders: number;
  refunded_orders: number;
  disputed_orders: number;
  failed_settlements: number;
};

export type EarningsPoint = {
  date: string;
  gross_cents: number;
  settled_cents: number;
  orders: number;
  settled_orders: number;
  refunded_cents: number;
};

export type EarningsByDataset = {
  dataset_id: string;
  title: string;
  total_orders: number;
  settled_orders: number;
  gross_cents: number;
  settled_cents: number;
  last_order_at: string;
};

export type Notification = {
  id: string;
  user_id: string;
  kind: string;
  title: string;
  body?: string;
  resource_type?: string;
  resource_id?: string;
  is_read: boolean;
  created_at?: string;
};

export type AuditLogEntry = {
  id: number;
  actor_id?: string;
  actor_role?: string;
  action: string;
  resource_type?: string;
  resource_id?: string;
  ip?: string;
  user_agent?: string;
  detail?: Record<string, unknown>;
  created_at: string;
};

export type Watch = {
  dataset_id: string;
  dataset_title?: string;
  last_notified_version_id?: string;
  created_at: string;
};

export type DatasetQuestion = {
  id: string;
  dataset_id: string;
  asker_id: string;
  asker_name?: string;
  body: string;
  status: string;
  answer?: { id: string; question_id: string; answerer_id: string; body: string; created_at: string };
  created_at: string;
};

export type Withdrawal = {
  id: string; seller_id: string; amount_cents: number;
  channel: string; account_label: string; status: string;
  ops_note?: string; requested_at: string;
  processed_at?: string; processed_by?: string;
};

export type Report = {
  id: string; reporter_id: string;
  target_type: "question" | "review"; target_id: string;
  reason: string; status: "open" | "resolved";
  resolution?: "hide" | "dismiss";
  created_at: string; resolved_at?: string; resolved_by?: string;
};

export type Anomaly = {
  id: string; kind: string; actor_id?: string;
  resource_pattern: string; sample_audit_ids: number[];
  count: number; first_seen_at: string; last_seen_at: string;
  status: string; ops_note?: string;
};

export type DataExportJob = {
  id: string; user_id: string; status: string;
  download_url?: string; object_bytes?: number;
  expires_at?: string; error?: string;
  requested_at: string; ready_at?: string;
};

export type DeletionRequest = {
  id: string; user_id: string; reason?: string;
  status: string; cooling_until: string;
  ops_note?: string; requested_at: string;
  processed_at?: string; processed_by?: string;
};

export type NotificationPreference = {
  kind: string;
  email_enabled: boolean;
  in_app_enabled: boolean;
};

export const tokenStore = {
  get access() {
    return typeof window === "undefined" ? null : localStorage.getItem(ACCESS_KEY);
  },
  get refresh() {
    return typeof window === "undefined" ? null : localStorage.getItem(REFRESH_KEY);
  },
  set(t: Tokens) {
    localStorage.setItem(ACCESS_KEY, t.access_token);
    localStorage.setItem(REFRESH_KEY, t.refresh_token);
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
};

type ReqOpts = {
  method?: string;
  body?: unknown;
  auth?: boolean;
  raw?: BodyInit; // raw body (e.g. upload part), skips JSON encoding
  query?: Record<string, string | number | undefined>;
  _retried?: boolean;
};

function buildURL(path: string, query?: ReqOpts["query"]): string {
  const url = new URL(BASE + path);
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
    }
  }
  return url.toString();
}

async function request<T>(path: string, opts: ReqOpts = {}): Promise<T> {
  const headers: Record<string, string> = {};
  const access = tokenStore.access;
  if (opts.auth !== false && access) headers["Authorization"] = `Bearer ${access}`;

  let body: BodyInit | undefined;
  if (opts.raw !== undefined) {
    body = opts.raw;
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }

  const res = await fetch(buildURL(path, opts.query), {
    method: opts.method ?? (body ? "POST" : "GET"),
    headers,
    body,
  });

  // Try a single refresh on auth failure, then retry the original request.
  if (res.status === 401 && opts.auth !== false && !opts._retried && tokenStore.refresh) {
    const refreshed = await tryRefresh();
    if (refreshed) return request<T>(path, { ...opts, _retried: true });
  }

  let json: { code: number; message: string; data: T } | null = null;
  try {
    json = await res.json();
  } catch {
    /* non-JSON (e.g. file download handled elsewhere) */
  }
  if (!res.ok || (json && json.code !== 0)) {
    throw new ApiError(json?.code ?? -1, res.status, json?.message ?? res.statusText);
  }
  return (json as { data: T }).data;
}

let refreshing: Promise<boolean> | null = null;
function tryRefresh(): Promise<boolean> {
  if (refreshing) return refreshing;
  refreshing = (async () => {
    try {
      const data = await request<AuthResult>("/auth/refresh", {
        method: "POST",
        body: { refresh_token: tokenStore.refresh },
        auth: false,
      });
      tokenStore.set(data.tokens);
      return true;
    } catch {
      tokenStore.clear();
      return false;
    } finally {
      refreshing = null;
    }
  })();
  return refreshing;
}

// --- typed API surface ---
export const api = {
  // auth
  register: (
    account: string,
    account_type: string,
    password: string,
    agreements?: { doc: string; version: string }[],
  ) =>
    request<AuthResult>("/auth/register", { body: { account, account_type, password, agreements }, auth: false }),
  listAgreements: () =>
    request<{ items: { doc: string; version: string; agreed_at: string }[] }>("/users/me/agreements"),
  recordAgreements: (agreements: { doc: string; version: string }[]) =>
    request<{ recorded: number }>("/users/me/agreements", { body: { agreements } }),
  login: (account: string, password: string) =>
    request<LoginResult>("/auth/login", { body: { account, password }, auth: false }),
  verify2FA: (challengeToken: string, code: string) =>
    request<{ user: User; tokens: Tokens }>("/auth/2fa/verify", { body: { challenge_token: challengeToken, code }, auth: false }),
  enroll2FA: () => request<Enroll2FAResult>("/auth/2fa/enroll"),
  verify2FAEnrollment: (code: string) =>
    request<{ ok: boolean }>("/auth/2fa/verify-enrollment", { body: { code } }),
  disable2FA: (code: string) =>
    request<{ ok: boolean }>("/auth/2fa/disable", { body: { code } }),
  recoveryCodeStatus: () =>
    request<{ remaining: number }>("/auth/2fa/recovery-status"),
  requestPasswordReset: (account: string) =>
    request<{ ok: boolean }>("/auth/password-reset/request", { body: { account }, auth: false }),
  completePasswordReset: (token: string, newPassword: string) =>
    request<{ ok: boolean }>("/auth/password-reset/complete", { body: { token, new_password: newPassword }, auth: false }),
  me: () => request<User>("/users/me"),
  updateRole: (role: string) => request<User>("/users/me", { method: "PUT", body: { role } }),
  getKYC: () => request<KYC>("/users/me/kyc"),
  submitKYC: (b: { type: string; real_name?: string; company_name?: string; id_no?: string; material_urls?: string[] }) =>
    request<KYC>("/users/me/kyc", { body: b }),

  // datasets
  listDatasets: (q: Record<string, string | number | undefined>) =>
    request<{ items: Dataset[] }>("/datasets", { auth: false, query: q }),
  // compute-to-data discovery signals for a batch of datasets (public catalog badge)
  computeOfferSignals: (datasetIds: string[]) =>
    request<{ signals: Record<string, ComputeSignal> }>("/compute/offers/signals", {
      auth: false,
      query: { dataset_ids: datasetIds.join(",") },
    }),
  getDataset: (id: string) => request<Dataset>(`/datasets/${id}`, { auth: false }),
  preview: (id: string) => request<Preview>(`/datasets/${id}/preview`),
  datasetReviews: (id: string) => request<{ items: Review[] }>(`/datasets/${id}/reviews`, { auth: false }),
  datasetQuality: (id: string) =>
    request<{ checks: QualityCheck[] }>(`/datasets/${id}/quality`, { auth: false }),
  datasetVersions: (id: string) =>
    request<{ versions: VersionInfo[] }>(`/datasets/${id}/versions`, { auth: false }),
  datasetCertificate: (id: string) =>
    request<Certificate>(`/datasets/${id}/certificate`, { auth: false }),
  // Absolute URL to the dataset's MLCommons Croissant JSON-LD (machine-readable).
  croissantUrl: (id: string) => `${BASE}/datasets/${id}/croissant`,
  myDatasets: () => request<{ items: Dataset[] }>("/users/me/datasets"),
  createDataset: (b: Record<string, unknown>) => request<Dataset>("/datasets", { body: b }),
  signSource: (id: string) => request<Dataset>(`/datasets/${id}/source-declaration/sign`, { method: "POST" }),
  updateDatasheet: (id: string, ds: Datasheet) =>
    request<Dataset>(`/datasets/${id}/datasheet`, { method: "PUT", body: ds }),

  // upload
  uploadInit: (id: string, filename: string) =>
    request<{ upload_id: string; object_key: string; suggested_part_size: number }>(
      `/datasets/${id}/upload/init`,
      { body: { filename } },
    ),
  uploadPart: (id: string, uploadId: string, partNumber: number, chunk: Blob) =>
    request<{ part_number: number; bytes: number }>(`/datasets/${id}/upload/part`, {
      method: "PUT",
      raw: chunk,
      query: { upload_id: uploadId, part_number: partNumber },
    }),
  uploadComplete: (id: string, uploadId: string) =>
    request<Dataset>(`/datasets/${id}/upload/complete`, { method: "POST", query: { upload_id: uploadId } }),

  // orders
  createOrder: (dataset_id: string, license_type: string) =>
    request<Order>("/orders", { body: { dataset_id, license_type } }),
  listOrders: (role?: string) => request<{ items: Order[] }>("/orders", { query: { role } }),
  getOrder: (id: string) => request<Order>(`/orders/${id}`),
  confirmDelivery: (id: string) => request<Order>(`/orders/${id}/confirm-delivery`, { method: "POST" }),
  dispute: (id: string, reason: string) => request<Order>(`/orders/${id}/dispute`, { body: { reason } }),
  review: (id: string, score: number, comment: string, issue_flag: boolean) =>
    request<Review>(`/orders/${id}/review`, { body: { score, comment, issue_flag } }),

  // payment + delivery
  createPayment: (order_id: string) =>
    request<{ pay_url: string; channel_txn_id: string; amount_cents: number; channel: string }>(
      "/payments/create",
      { body: { order_id } },
    ),
  devMarkPaid: (order_id: string) => request<{ status: string }>("/payments/dev/mark-paid", { body: { order_id } }),
  requestDownload: (id: string) =>
    request<{ download_url: string; expires_at: string }>(`/orders/${id}/download`, {
      body: { license_agreed: true },
    }),

  // seller / ops
  earnings: () => request<Earnings>("/sellers/me/earnings"),
  adminTransactions: () => request<{ items: Order[] }>("/admin/transactions"),
  adminListDatasets: (status: string) =>
    request<{ items: Dataset[] }>("/admin/datasets", { query: { status } }),
  adminReviewDataset: (id: string, approve: boolean, note: string) =>
    request<Dataset>(`/admin/datasets/${id}/review`, { body: { approve, note } }),
  adminListKYC: () => request<{ items: KYC[] }>("/admin/kyc/pending"),
  adminReviewKYC: (kyc_id: string, approve: boolean) =>
    request<KYC>("/admin/kyc/review", { body: { kyc_id, approve } }),
  adminResolveDispute: (id: string, refund: boolean, note: string) =>
    request<Order>(`/admin/orders/${id}/resolve`, { body: { refund, note } }),

  // admin compute-to-data
  adminListComputeAlgorithms: (status?: string) =>
    request<{ items: ComputeAlgorithm[] }>("/admin/compute/algorithms", { query: { status } }),
  adminRegisterAlgorithm: (b: {
    name: string; runtime: string; image: string; image_digest: string;
    version: number; source_ref: string; entrypoint: string; output_kind: string;
    params_schema?: Record<string, unknown>;
  }) => request<ComputeAlgorithm>("/admin/compute/algorithms", { method: "POST", body: b }),
  adminReviewAlgorithm: (id: string, status: string, trusted: boolean) =>
    request<ComputeAlgorithm>(`/admin/compute/algorithms/${id}/review`, { body: { status, trusted } }),
  adminListComputeJobs: (status?: string, limit?: number) =>
    request<{ items: ComputeJob[] }>("/admin/compute/jobs", { query: { status, limit } }),
  adminReleaseComputeJob: (id: string) =>
    request<ComputeJob>(`/admin/compute/jobs/${id}/release`, { method: "POST" }),
  adminRejectComputeJob: (id: string, reason: string) =>
    request<ComputeJob>(`/admin/compute/jobs/${id}/reject`, { body: { reason } }),

  // admin settlement outbox
  adminListSettlementOutbox: (status?: string, limit?: number, offset?: number) =>
    request<{ items: OutboxEntry[] }>("/admin/settlement-outbox", { query: { status, limit, offset } }),
  adminRetrySettlementOutbox: (orderId: string) =>
    request<{ order_id: string; status: string }>(`/admin/settlement-outbox/${orderId}/retry`, { method: "POST" }),

  // admin reconciliation
  adminReconciliation: () =>
    request<{
      total_gmv: number; settled_gmv: number; platform_fees: number;
      total_orders: number; settled_orders: number; pending_orders: number;
      disputed_orders: number; refunded_orders: number; refunded_amount: number;
      failed_settlements: number;
    }>("/admin/reconciliation"),
  adminReconciliationTimeseries: (days?: number) =>
    request<{ days: number; from: string; to: string; points: ReconciliationPoint[] }>(
      "/admin/reconciliation/timeseries", { query: { days } }),
  sellerEarningsTimeseries: (days?: number) =>
    request<{ days: number; from: string; to: string; points: EarningsPoint[] }>(
      "/sellers/me/earnings/timeseries", { query: { days } }),
  sellerEarningsByDataset: () =>
    request<{ items: EarningsByDataset[] }>("/sellers/me/earnings/by-dataset"),

  // notifications
  listNotifications: (limit?: number, offset?: number) =>
    request<{ items: Notification[] }>("/users/me/notifications", { query: { limit, offset } }),
  countUnreadNotifications: () =>
    request<{ unread: number }>("/users/me/notifications/unread-count"),
  markNotificationRead: (id: string) =>
    request<{ ok: boolean }>(`/users/me/notifications/${id}/read`, { method: "POST" }),
  markAllNotificationsRead: () =>
    request<{ marked: number }>("/users/me/notifications/read-all", { method: "POST" }),

  // notification preferences
  getNotificationPreferences: () =>
    request<{ items: Record<string, { kind: string; email_enabled: boolean; in_app_enabled: boolean }> }>(
      "/users/me/notification-preferences"),
  updateNotificationPreference: (kind: string, email: boolean, inApp: boolean) =>
    request<NotificationPreference>("/users/me/notification-preferences", {
      method: "PUT", body: { kind, email_enabled: email, in_app_enabled: inApp },
    }),

  // certificate verification (public)
  verifyCertificate: (certId: string) =>
    request<{
      cert_id: string; resource_type: string; resource_id: string;
      registered_at: string; status: string; verifiable: boolean;
      statement_zh: string; statement_en: string;
    }>(`/verify/${certId}`, { auth: false }),

  // watchlist
  watchDataset:   (id: string) => request<{ ok: boolean }>(`/datasets/${id}/watch`, { method: "POST" }),
  unwatchDataset: (id: string) => request<{ ok: boolean }>(`/datasets/${id}/watch`, { method: "DELETE" }),
  listMyWatches:  () => request<{ items: Watch[] }>("/users/me/watched"),

  // dataset Q&A
  listDatasetQuestions: (id: string, limit?: number, offset?: number) =>
    request<{ items: DatasetQuestion[] }>(`/datasets/${id}/questions`, {
      query: { limit, offset }, auth: false,
    }),
  askDatasetQuestion: (id: string, body: string) =>
    request<DatasetQuestion>(`/datasets/${id}/questions`, { body: { body } }),
  answerQuestion: (qid: string, body: string) =>
    request<{ id: string; question_id: string; body: string; created_at: string }>(
      `/questions/${qid}/answer`, { body: { body } }),

  // withdrawal (book-keeping, P module)
  requestWithdrawal: (b: { amount_cents: number; channel: string; account_label: string }) =>
    request<Withdrawal>("/sellers/me/withdrawals", { body: b }),
  listMyWithdrawals: (limit?: number, offset?: number) =>
    request<{ items: Withdrawal[] }>("/sellers/me/withdrawals", { query: { limit, offset } }),
  adminListWithdrawals: (status?: string, limit?: number, offset?: number) =>
    request<{ items: Withdrawal[] }>("/admin/withdrawals", { query: { status, limit, offset } }),
  adminApproveWithdrawal: (id: string, note?: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/approve`, { body: { note } }),
  adminRejectWithdrawal: (id: string, reason: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/reject`, { body: { reason } }),
  adminCompleteWithdrawal: (id: string, note?: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/complete`, { body: { note } }),

  // content moderation (ops) — unified report queue (questions + reviews)
  adminListReports: (status?: string, limit?: number, offset?: number) =>
    request<{ items: Report[] }>("/admin/reports", { query: { status, limit, offset } }),
  adminResolveReport: (id: string, action: "hide" | "dismiss") =>
    request<Report>(`/admin/reports/${id}/resolve`, { body: { action } }),

  // anomaly detection (ops)
  adminListAnomalies: (status?: string, limit?: number, offset?: number) =>
    request<{ items: Anomaly[] }>("/admin/anomalies", { query: { status, limit, offset } }),
  adminAcknowledgeAnomaly: (id: string, note?: string) =>
    request<Anomaly>(`/admin/anomalies/${id}/acknowledge`, { body: { note } }),
  adminResolveAnomaly: (id: string, note?: string) =>
    request<Anomaly>(`/admin/anomalies/${id}/resolve`, { body: { note } }),

  // compliance — data export + account deletion
  requestDataExport: () => request<DataExportJob>("/users/me/data-export", { body: {} }),
  getMyDataExport: () => request<DataExportJob>("/users/me/data-export"),
  downloadMyDataExport: async () => {
    const res = await fetch(buildURL("/users/me/data-export/download"), {
      headers: tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {},
    });
    if (!res.ok) throw new ApiError(-1, res.status, "下载失败");
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `oasis-data-export-${Date.now()}.zip`;
    document.body.appendChild(a);
    a.click(); a.remove();
    URL.revokeObjectURL(url);
  },
  requestAccountDeletion: (reason: string) =>
    request<DeletionRequest>("/users/me/account/deletion", { body: { reason } }),
  cancelAccountDeletion: () =>
    request<{ ok: boolean; id: string; status: string }>("/users/me/account/deletion", { method: "DELETE" }),
  adminListDeletions: (status?: string, limit?: number, offset?: number) =>
    request<{ items: DeletionRequest[] }>("/admin/account-deletions", { query: { status, limit, offset } }),
  adminApproveDeletion: (id: string, note?: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/approve`, { body: { note } }),
  adminRejectDeletion: (id: string, reason: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/reject`, { body: { reason } }),
  adminExecuteDeletion: (id: string, note?: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/execute`, { body: { note } }),

  // audit logs (ops)
  adminListAuditLogs: (q: {
    actor?: string;
    action?: string;
    resource_type?: string;
    resource_id?: string;
    from?: string;
    to?: string;
    limit?: number;
    offset?: number;
  }) => request<{ items: AuditLogEntry[]; limit: number; offset: number; next_offset?: number }>(
    "/admin/audit-logs",
    { query: q as Record<string, string | number | undefined> },
  ),

  // compute-to-data (C2D / 可用不可见)
  getComputeOffer: (id: string) =>
    request<ComputeOffer>(`/datasets/${id}/compute-offer`, { auth: false }),
  putComputeOffer: (id: string, body: Partial<ComputeOffer>) =>
    request<ComputeOffer>(`/datasets/${id}/compute-offer`, { method: "PUT", body }),
  listComputeAlgorithms: (dataset_id: string) =>
    request<{ items: ComputeAlgorithm[] }>("/compute/algorithms", { query: { dataset_id } }),
  purchaseCompute: (id: string, quota: number) =>
    request<ComputeEntitlement>(`/datasets/${id}/compute/purchase`, { body: { quota } }),
  // Real purchase: create a compute order, then pay it via the order page.
  createComputeOrder: (id: string) =>
    request<{ order_id: string }>(`/datasets/${id}/compute/order`, { method: "POST" }),
  listMyComputeEntitlements: () =>
    request<{ items: ComputeEntitlement[] }>("/users/me/compute/entitlements"),
  submitComputeJob: (b: { dataset_id: string; entitlement_id: string; algorithm_id: string; params?: Record<string, unknown> }) =>
    request<ComputeJob>("/compute/jobs", { body: b }),
  getComputeJob: (id: string) => request<ComputeJob>(`/compute/jobs/${id}`),
  getComputeAttestation: (id: string) =>
    request<ComputeAttestation>(`/compute/jobs/${id}/attestation`),
  getComputeJobCertificate: (id: string) =>
    request<Record<string, unknown>>(`/compute/jobs/${id}/certificate`),
  getFederatedCertificate: (id: string) =>
    request<Record<string, unknown>>(`/compute/federated-jobs/${id}/certificate`),
  listMyComputeJobs: () => request<{ items: ComputeJob[] }>("/users/me/compute/jobs"),
  cancelComputeJob: (id: string) => request<ComputeJob>(`/compute/jobs/${id}/cancel`, { method: "POST" }),
  // Custom-algorithm submission: forced pending + untrusted server-side; appears
  // in the ops review queue. Cannot run until approved.
  requestAlgorithm: (b: {
    name: string;
    runtime: string;
    image: string;
    output_kind: string;
    image_digest?: string;
    source_ref?: string;
    entrypoint?: string;
  }) => request<ComputeAlgorithm>("/compute/algorithm-requests", { body: b }),
  listMyAlgorithmRequests: () =>
    request<{ items: ComputeAlgorithm[] }>("/users/me/compute/algorithm-requests"),
  // The output endpoint streams raw bytes (auth-gated); fetch with the bearer
  // token and trigger a browser download.
  downloadComputeOutput: async (id: string) => {
    const res = await fetch(buildURL(`/compute/jobs/${id}/output`), {
      headers: tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {},
    });
    if (!res.ok) throw new ApiError(-1, res.status, "下载失败");
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `compute-output-${id}.bin`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },

  // Bundle download: zip multiple settled download orders into a single file.
  bundleDownload: async (orderIds: string[]) => {
    const res = await fetch(buildURL("/users/me/orders/bundle"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {}),
      },
      body: JSON.stringify({ order_ids: orderIds }),
    });
    if (!res.ok) {
      let msg = "打包下载失败";
      try {
        const j = await res.json();
        msg = j.message || msg;
      } catch { /* non-json body */ }
      throw new ApiError(-1, res.status, msg);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `oasis-bundle-${Date.now()}.zip`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },

  // federated (L3) compute
  submitFederatedJob: (b: {
    algorithm_id: string;
    dataset_ids: string[];
    min_participants?: number;
    dp_epsilon?: number;
    params?: Record<string, unknown>;
  }) => request<FederatedJob>("/compute/federated-jobs", { body: b }),
  getFederatedJob: (id: string) =>
    request<{ federated_job: FederatedJob; sub_jobs: ComputeJob[] }>(`/compute/federated-jobs/${id}`),
  listMyFederatedJobs: (limit?: number, offset?: number) =>
    request<{ items: FederatedJob[] }>("/users/me/compute/federated-jobs", { query: { limit, offset } }),
  downloadFederatedOutput: async (id: string) => {
    const res = await fetch(buildURL(`/compute/federated-jobs/${id}/output`), {
      headers: tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {},
    });
    if (!res.ok) throw new ApiError(-1, res.status, "下载失败");
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `federated-model-${id}.json`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },
  // Fetch a federated job's joint output as parsed JSON (used to show a PSI
  // intersection inline rather than forcing a download).
  getFederatedOutputJSON: async <T = unknown>(id: string): Promise<T> => {
    const res = await fetch(buildURL(`/compute/federated-jobs/${id}/output`), {
      headers: tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {},
    });
    if (!res.ok) throw new ApiError(-1, res.status, "获取结果失败");
    return (await res.json()) as T;
  },
};

export function yuan(cents?: number): string {
  if (cents === undefined || cents === null) return "—";
  return "¥" + (cents / 100).toFixed(2);
}
