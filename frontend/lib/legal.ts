// Current legal-document versions the UI asks users to accept. Bump a version
// when its text materially changes; the backend records consent per (doc,
// version) so an update can trigger re-consent. Values are placeholders until
// counsel finalizes the texts (see docs/legal/).
export const LEGAL_VERSIONS = {
  terms: "v0.1-draft",
  privacy: "v0.1-draft",
} as const;

export type AgreementInput = { doc: string; version: string };

// Agreements captured at sign-up.
export const SIGNUP_AGREEMENTS: AgreementInput[] = [
  { doc: "terms", version: LEGAL_VERSIONS.terms },
  { doc: "privacy", version: LEGAL_VERSIONS.privacy },
];
