// Current legal-document versions the UI asks users to accept. Bump a version
// when its text materially changes; the backend records consent per (doc,
// version) so an update can trigger re-consent. Texts finalized per counsel's
// 2026-06-01 review (see docs/legal/); a few business slots remain (custodian
// name, ICP/EDI licensing) but the agreement text is in effect.
export const LEGAL_VERSIONS = {
  terms: "v1.0-2026-06-01",
  privacy: "v1.0-2026-06-01",
} as const;

export type AgreementInput = { doc: string; version: string };

// Agreements captured at sign-up.
export const SIGNUP_AGREEMENTS: AgreementInput[] = [
  { doc: "terms", version: LEGAL_VERSIONS.terms },
  { doc: "privacy", version: LEGAL_VERSIONS.privacy },
];
