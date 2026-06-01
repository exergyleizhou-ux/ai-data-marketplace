// Package quality orchestrates asynchronous quality checks on uploaded data:
// format/encoding validation, basic statistics, dedup (SHA256 exact +
// MinHash/SimHash near-dup), PII scanning, and post-redaction verification.
//
// PII detection (pii.go) shares one engine across detect / mask / verify:
//   - validated high-precision detectors: 身份证 (mod-11 checksum), 银行卡 (Luhn),
//     手机号, 邮箱, IPv4 (octet range);
//   - heuristic detectors: 护照, 车牌, GPS 坐标, 地址.
//
// MaskPII redacts only the matched span (never flanking text) and PIIRedaction
// proves de-identification leaves zero residual — the trust artifact behind the
// ToS §5.1 de-identification warranty.
//
// Boundary: owns `quality_check`. Checks run in async workers (Asynq) and must
// stream large files — never load a whole file into memory.
//
// Implemented in: PR-09 (basic checks + write-back + state advance).
package quality
