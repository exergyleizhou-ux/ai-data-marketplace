package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// certificateID is the dataset's "一数一码" — a stable, recomputable code derived
// from the dataset id and its content fingerprint. Deterministic so anyone can
// reproduce and verify it.
func certificateID(datasetID, contentSHA string) string {
	sum := sha256.Sum256([]byte(datasetID + ":" + contentSHA))
	return "VO-" + strings.ToUpper(hex.EncodeToString(sum[:])[:12])
}

// BuildCertificate assembles a data integrity & registration certificate — the
// "数据产品登记凭证 / 存证" pattern from China's data exchanges, adapted honestly:
// a platform-issued record over the content SHA-256 + registration time, not (yet)
// third-party/blockchain notarized. Pure function — no I/O — so it is unit-tested.
func BuildCertificate(d Dataset, vm VersionMeta, checks []QualityCheck) map[string]any {
	if vm.ContentSHA256 == "" {
		return map[string]any{
			"status":       "pending",
			"dataset_id":   d.ID,
			"statement_zh": "数据尚未上传，暂无存证凭证。",
			"statement_en": "No content uploaded yet — certificate pending.",
		}
	}
	cert := map[string]any{
		"status":         "registered",
		"certificate_id": certificateID(d.ID, vm.ContentSHA256),
		"dataset_id":     d.ID,
		"title":          d.Title,
		"operator":       "杭州科农绿洲生物科技有限公司",
		"content_sha256": vm.ContentSHA256,
		"version_no":     vm.VersionNo,
		"integrity":      map[string]any{"algorithm": "SHA-256", "verifiable": true},
		"quality":        certQuality(checks),
		"statement_zh": "本凭证由平台基于数据内容指纹（SHA-256）与登记时间出具，用于数据完整性校验与登记存证（一数一码）。" +
			"买方可对下载数据重新计算 SHA-256 与本凭证比对以验证完整性。本凭证为平台自行出具，尚未接入第三方公证或区块链存证。",
		"statement_en": "Platform-issued integrity & registration record over the content fingerprint (SHA-256) and " +
			"registration time. Buyers can re-hash the downloaded data and compare. Not yet third-party/blockchain notarized.",
	}
	if d.CreatedAt != "" {
		cert["registered_at"] = d.CreatedAt
	}
	return cert
}

// certQuality surfaces the headline quality signals inside the certificate.
func certQuality(checks []QualityCheck) map[string]any {
	q := map[string]any{}
	for _, c := range checks {
		switch c.Type {
		case "authenticity":
			if applicable, _ := c.Report["applicable"].(bool); applicable {
				if band, ok := c.Report["band"].(string); ok && band != "" {
					q["authenticity_band"] = band
				}
			}
		case "pii_redaction":
			if verified, _ := c.Report["verified"].(bool); verified {
				q["pii_deidentified"] = "verified-zero-residual"
			}
		}
	}
	return q
}
