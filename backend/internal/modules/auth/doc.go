// Package auth owns users, KYC (实名认证), sessions/JWT and RBAC.
//
// Boundary: this package owns the `user`, `kyc_record` and `payout_account`
// tables. Other modules obtain identity/role only through this package's
// exported interfaces — never by querying its tables directly.
//
// Implemented in: PR-04 (register/login/JWT), PR-05 (profile/KYC), PR-06 (RBAC).
package auth
