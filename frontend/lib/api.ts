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
};
export type AuthResult = { user: User; tokens: Tokens };

export type SourceDeclaration = {
  source: string;
  collection_method: string;
  contains_pii: boolean;
  license_scope: string;
  commitment: boolean;
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
    request<AuthResult>("/auth/login", { body: { account, password }, auth: false }),
  me: () => request<User>("/users/me"),
  updateRole: (role: string) => request<User>("/users/me", { method: "PUT", body: { role } }),
  getKYC: () => request<KYC>("/users/me/kyc"),
  submitKYC: (b: { type: string; real_name?: string; company_name?: string; id_no?: string; material_urls?: string[] }) =>
    request<KYC>("/users/me/kyc", { body: b }),

  // datasets
  listDatasets: (q: Record<string, string | number | undefined>) =>
    request<{ items: Dataset[] }>("/datasets", { auth: false, query: q }),
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
  listMyComputeJobs: () => request<{ items: ComputeJob[] }>("/users/me/compute/jobs"),
  cancelComputeJob: (id: string) => request<ComputeJob>(`/compute/jobs/${id}/cancel`, { method: "POST" }),
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
};

export function yuan(cents?: number): string {
  if (cents === undefined || cents === null) return "—";
  return "¥" + (cents / 100).toFixed(2);
}
