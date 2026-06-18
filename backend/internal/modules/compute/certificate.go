package compute

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// jobCertificateID is a compute result's "一码" — a stable, recomputable code
// derived from the job id and its output fingerprint. Deterministic so anyone can
// reproduce and verify it (mirrors the dataset 一数一码 / 存证 pattern).
func jobCertificateID(jobID, outputSHA string) string {
	sum := sha256.Sum256([]byte(jobID + ":" + outputSHA))
	return "VO-" + strings.ToUpper(hex.EncodeToString(sum[:])[:12])
}

// BuildJobCertificate assembles a compute-result provenance & registration
// certificate: the data-exchange 存证 pattern (PR #26) extended from datasets to
// compute-to-data RESULTS. It binds the released output's SHA-256 to the audited
// code that produced it (algorithm + pinned image digest) and the source dataset —
// so a buyer can prove "this model/statistic came from algorithm X over dataset Y",
// and re-hash the downloaded output to verify integrity. Pure function (no I/O):
// the caller computes outputSHA from the stored output and passes it in.
//
// Honest scope: platform-issued provenance over the content fingerprint, not (yet)
// third-party/blockchain notarized — same stance as the dataset certificate.
func BuildJobCertificate(job Job, algo Algorithm, outputSHA string) map[string]any {
	if job.Status != JobReleased || outputSHA == "" {
		return map[string]any{
			"status":       "pending",
			"job_id":       job.ID,
			"statement_zh": "计算结果尚未放行，暂无存证凭证。",
			"statement_en": "Compute result not released yet — certificate pending.",
		}
	}
	cert := map[string]any{
		"status":         "registered",
		"certificate_id": jobCertificateID(job.ID, outputSHA),
		"job_id":         job.ID,
		"dataset_id":     job.DatasetID,
		"operator":       "杭州科农绿洲生物科技有限公司",
		"output_kind":    job.OutputKind,
		"output_sha256":  outputSHA,
		"output_bytes":   job.OutputBytes,
		"integrity":      map[string]any{"algorithm": "SHA-256", "verifiable": true},
		"algorithm": map[string]any{
			"id":           algo.ID,
			"name":         algo.Name,
			"version":      job.AlgorithmVersion, // the version PINNED at submit, not the live (mutable) algo row
			"image_digest": algo.ImageDigest,     // immutable post-register: the audited code that produced the result
			"trusted":      algo.Trusted,
		},
		"statement_zh": "本凭证由平台基于「可用不可见」计算结果的内容指纹（SHA-256）、产出该结果的已审核算法（镜像 digest 钉死）" +
			"与源数据集出具,用于结果完整性校验与计算溯源存证。买方可对下载结果重新计算 SHA-256 与本凭证比对。" +
			"本凭证为平台自行出具,尚未接入第三方公证或区块链存证。",
		"statement_en": "Platform-issued provenance & integrity record for a compute-to-data result: it binds the output " +
			"fingerprint (SHA-256) to the audited algorithm (pinned image digest) that produced it and the source dataset. " +
			"Buyers can re-hash the downloaded result and compare. Not yet third-party/blockchain notarized.",
	}
	if job.FinishedAt != "" {
		cert["registered_at"] = job.FinishedAt
	}
	if job.Attestation != nil {
		cert["confidential"] = map[string]any{"attested": true, "note": "L2 TEE remote attestation present"}
	}
	return cert
}

// BuildFederatedCertificate is the joint-result counterpart of BuildJobCertificate
// for L3 jobs (federated learning or PSI): it binds the joint output's SHA-256 to
// the audited algorithm and ALL participating datasets (the parties), recording the
// mode so the certificate states whether it certifies a federated joint model or a
// private set intersection. Raw data never left any party's sandbox; this certifies
// the aggregate result's provenance. Pure function.
func BuildFederatedCertificate(fed FederatedJob, algo Algorithm, outputSHA string) map[string]any {
	if fed.Status != FedReleased || outputSHA == "" {
		return map[string]any{
			"status":           "pending",
			"federated_job_id": fed.ID,
			"statement_zh":     "联合计算结果尚未放行，暂无存证凭证。",
			"statement_en":     "Joint result not released yet — certificate pending.",
		}
	}
	cert := map[string]any{
		"status":           "registered",
		"certificate_id":   jobCertificateID(fed.ID, outputSHA),
		"federated_job_id": fed.ID,
		"mode":             fed.Mode,
		"dataset_ids":      fed.DatasetIDs,
		"parties":          len(fed.DatasetIDs),
		"operator":         "杭州科农绿洲生物科技有限公司",
		"output_sha256":    outputSHA,
		"output_bytes":     fed.OutputBytes,
		"integrity":        map[string]any{"algorithm": "SHA-256", "verifiable": true},
		"algorithm": map[string]any{
			"id":           algo.ID,
			"name":         algo.Name,
			"version":      algo.Version,
			"image_digest": algo.ImageDigest,
			"trusted":      algo.Trusted,
		},
		"statement_zh": "本凭证由平台基于「数据不出域」联合计算结果的内容指纹（SHA-256）、产出该结果的已审核算法" +
			"（镜像 digest 钉死）与各参与数据集出具,用于联合结果完整性校验与计算溯源存证。原始数据始终未出各方沙箱。" +
			"本凭证为平台自行出具,尚未接入第三方公证或区块链存证。",
		"statement_en": "Platform-issued provenance & integrity record for a data-stays-home JOINT result (federated " +
			"learning or PSI): it binds the joint output fingerprint (SHA-256) to the audited algorithm (pinned image " +
			"digest) and all participating datasets. Raw data never left any party's sandbox. Not yet third-party/blockchain notarized.",
	}
	if fed.DPEpsilon != nil {
		cert["differential_privacy"] = map[string]any{"epsilon": *fed.DPEpsilon}
	}
	if fed.UpdatedAt != "" {
		cert["registered_at"] = fed.UpdatedAt
	}
	return cert
}
