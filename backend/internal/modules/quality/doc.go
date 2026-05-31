// Package quality orchestrates asynchronous quality checks on uploaded data:
// format/encoding validation, basic statistics, dedup (SHA256 exact +
// MinHash/SimHash near-dup), and PII scanning (身份证/手机号/邮箱/银行卡/地址).
//
// Boundary: owns `quality_check`. Checks run in async workers (Asynq) and must
// stream large files — never load a whole file into memory.
//
// Implemented in: PR-09 (basic checks + write-back + state advance).
package quality
