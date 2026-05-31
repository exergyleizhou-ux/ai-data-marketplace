// Package delivery owns secure data hand-off to buyers.
//
// Reality (see docs §2.3): plain text / code / JSON / CSV cannot be
// meaningfully watermarked — once downloaded it can be copied infinitely. We do
// NOT promise "anti-piracy"; we promise "traceable + accountable":
//   - law (primary): mandatory license e-signature before download
//   - delivery fingerprint (weak): per buyer+order salted hash, recorded
//   - one-time, short-lived presigned URLs (e.g. 15 min), download audited
//   - API/sandbox delivery is the real defense — deferred to P1
//
// Boundary: owns `delivery`. Implemented in: PR-14.
package delivery
