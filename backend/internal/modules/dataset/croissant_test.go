package dataset

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func sampleDataset() Dataset {
	return Dataset{
		ID:          "ds-42",
		Title:       "中文清洗语料 Clean Chinese Corpus",
		Description: "A cleaned tabular dataset.",
		DataType:    "structured",
		Domain:      "finance",
		LicenseType: licenseCommercial,
		CreatedAt:   "2026-06-02T00:00:00Z",
	}
}

func sampleVM() VersionMeta {
	return VersionMeta{VersionNo: 2, ContentSHA256: "abc123", ObjectKey: "datasets/ds-42/data.csv", SizeBytes: 1024, ContentType: "text/csv"}
}

func TestBuildCroissantCoreShape(t *testing.T) {
	doc := BuildCroissant(sampleDataset(), sampleVM(), nil, "https://oasis.example")

	// Round-trips as JSON (no un-marshalable values).
	if _, err := json.Marshal(doc); err != nil {
		t.Fatalf("doc must be JSON-serializable: %v", err)
	}
	if doc["conformsTo"] != "http://mlcommons.org/croissant/1.0" {
		t.Errorf("conformsTo = %v", doc["conformsTo"])
	}
	if doc["@type"] != "sc:Dataset" {
		t.Errorf("@type = %v", doc["@type"])
	}
	ctx, ok := doc["@context"].(map[string]any)
	if !ok || ctx["cr"] != "http://mlcommons.org/croissant/" || ctx["sc"] != "https://schema.org/" {
		t.Errorf("@context missing cr/sc vocab: %v", doc["@context"])
	}
	if doc["name"] != "中文清洗语料 Clean Chinese Corpus" {
		t.Errorf("name = %v", doc["name"])
	}
	if !strings.Contains(doc["url"].(string), "/datasets/ds-42") {
		t.Errorf("url = %v", doc["url"])
	}
	if doc["version"] != "2.0" {
		t.Errorf("version = %v, want 2.0", doc["version"])
	}
	if !strings.Contains(doc["license"].(string), "Commercial") {
		t.Errorf("license = %v", doc["license"])
	}
}

func TestBuildCroissantDistribution(t *testing.T) {
	doc := BuildCroissant(sampleDataset(), sampleVM(), nil, "https://oasis.example")
	dist, ok := doc["distribution"].([]any)
	if !ok || len(dist) != 1 {
		t.Fatalf("distribution = %v", doc["distribution"])
	}
	fo := dist[0].(map[string]any)
	if fo["@type"] != "cr:FileObject" {
		t.Errorf("file @type = %v", fo["@type"])
	}
	if fo["encodingFormat"] != "text/csv" {
		t.Errorf("encodingFormat = %v", fo["encodingFormat"])
	}
	if fo["contentSize"] != "1024 B" {
		t.Errorf("contentSize = %v", fo["contentSize"])
	}
	if fo["sha256"] != "abc123" {
		t.Errorf("sha256 = %v", fo["sha256"])
	}
	if fo["name"] != "data.csv" {
		t.Errorf("name = %v", fo["name"])
	}
}

func TestBuildCroissantQualityProperties(t *testing.T) {
	checks := []QualityCheck{
		{Type: "authenticity", Result: "warn", Report: map[string]any{"applicable": true, "band": "review", "score": float64(72)}},
		{Type: "pii_redaction", Result: "pass", Report: map[string]any{"verified": true}},
	}
	doc := BuildCroissant(sampleDataset(), sampleVM(), checks, "https://oasis.example")
	props := doc["additionalProperty"].([]any)
	got := map[string]string{}
	for _, p := range props {
		m := p.(map[string]any)
		got[m["name"].(string)] = m["value"].(string)
	}
	if got["authenticity_band"] != "review" {
		t.Errorf("authenticity_band = %q", got["authenticity_band"])
	}
	if got["authenticity_score"] != "72" {
		t.Errorf("authenticity_score = %q", got["authenticity_score"])
	}
	if got["pii_deidentified"] != "verified-zero-residual" {
		t.Errorf("pii_deidentified = %q", got["pii_deidentified"])
	}
	if _, ok := got["quality_screened_by"]; !ok {
		t.Errorf("missing quality_screened_by")
	}
}

func TestBuildCroissantOmitsNonApplicableAuthenticity(t *testing.T) {
	// A text dataset's authenticity check is applicable=false — it must NOT emit
	// an authenticity_band property (no false "clean" claim).
	checks := []QualityCheck{
		{Type: "authenticity", Result: "pass", Report: map[string]any{"applicable": false, "band": "clean", "score": float64(100)}},
	}
	doc := BuildCroissant(sampleDataset(), sampleVM(), checks, "https://oasis.example")
	if props, ok := doc["additionalProperty"].([]any); ok {
		for _, p := range props {
			if p.(map[string]any)["name"] == "authenticity_band" {
				t.Errorf("non-applicable authenticity must not surface a band: %v", props)
			}
		}
	}
}

func TestCroissantMetadataService(t *testing.T) {
	repo := newFakeRepo()
	repo.items["ds-1"] = Dataset{ID: "ds-1", Title: "T", Status: StatusPublished, DataType: "text", LicenseType: licenseResearch}
	svc := NewService(repo, fakeIdentity{status: map[string]string{}}, nil)

	doc, err := svc.CroissantMetadata(context.Background(), "ds-1", "https://oasis.example")
	if err != nil {
		t.Fatalf("CroissantMetadata: %v", err)
	}
	if doc["conformsTo"] != "http://mlcommons.org/croissant/1.0" {
		t.Errorf("conformsTo = %v", doc["conformsTo"])
	}
	if !strings.Contains(doc["license"].(string), "Research") {
		t.Errorf("license = %v", doc["license"])
	}

	if _, err := svc.CroissantMetadata(context.Background(), "missing", "https://x"); err == nil {
		t.Errorf("unknown dataset should error")
	}
}
