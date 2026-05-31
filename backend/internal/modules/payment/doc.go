// Package payment owns payment orders and split-settlement (分账) orchestration.
//
// CRITICAL COMPLIANCE RULE (see docs §2.1): the platform NEVER holds user
// funds in its own account. Funds are escrowed at a licensed provider
// (财付通/支付宝 or a licensed aggregator); the platform only issues split
// instructions. Collecting then redistributing funds via a platform account is
// "资金二清" — a criminal-liability red line, not a compliance nitpick.
//
// Boundary: owns `payment`, `settlement`. Engineering invariants:
//   - webhook callbacks: idempotent (dedup on channel_txn_id) + signature-verified
//   - confirmed->settled: distributed lock + settlement.idempotency_key unique
//   - reliable delivery: outbox pattern for split tasks
//
// Implemented in: PR-12 (WeChat pay + idempotent webhook), PR-13 (split+settle),
// PR-15 (Alipay/Stripe), PR-17 (seller earnings).
package payment
