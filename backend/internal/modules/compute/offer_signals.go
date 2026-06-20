package compute

import "context"

// OfferSignal is the public compute-to-data discovery signal for one dataset: it
// tells a browsing buyer that the dataset supports verifiable sandbox compute, at
// what trust level, whether federated/PSI is allowed, and how many results have
// already been produced (a usage/confidence cue). It carries NO sensitive data —
// the trust level and flags are public offer config, the count is an aggregate.
type OfferSignal struct {
	DatasetID      string `json:"dataset_id"`
	Enabled        bool   `json:"enabled"`
	TrustLevel     string `json:"trust_level"`
	AllowFederated bool   `json:"allow_federated"`
	AllowPSI       bool   `json:"allow_psi"`
	JobsRun        int    `json:"jobs_run"`
}

// OfferSignals batches the discovery signal for many datasets in one query
// (avoids N+1 on the catalog). Only datasets with an enabled offer are returned.
func (r *pgRepo) OfferSignals(ctx context.Context, datasetIDs []string) (map[string]OfferSignal, error) {
	out := make(map[string]OfferSignal)
	if len(datasetIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT o.dataset_id, o.enabled, o.trust_level, o.allow_federated, o.allow_psi,
		       COALESCE(j.cnt, 0)
		FROM dataset_compute_offers o
		LEFT JOIN (
			SELECT dataset_id, count(*) AS cnt
			FROM compute_jobs WHERE status = 'released' GROUP BY dataset_id
		) j ON j.dataset_id = o.dataset_id
		WHERE o.dataset_id = ANY($1) AND o.enabled = true`, datasetIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s OfferSignal
		if err := rows.Scan(&s.DatasetID, &s.Enabled, &s.TrustLevel, &s.AllowFederated, &s.AllowPSI, &s.JobsRun); err != nil {
			return nil, err
		}
		out[s.DatasetID] = s
	}
	return out, rows.Err()
}

// OfferSignals exposes the batch discovery signal through the service layer.
func (s *Service) OfferSignals(ctx context.Context, datasetIDs []string) (map[string]OfferSignal, error) {
	return s.repo.OfferSignals(ctx, datasetIDs)
}
