package dataset

import (
	"fmt"
	"path"
	"strings"
)

// croissantContext is the standard MLCommons Croissant 1.0 JSON-LD context
// (https://docs.mlcommons.org/croissant/). Kept verbatim so the export is
// recognised by Croissant-aware loaders (HF datasets, TFDS, Google Dataset
// Search). Trimmed to the terms this export actually uses.
func croissantContext() map[string]any {
	return map[string]any{
		"@language":          "en",
		"@vocab":             "https://schema.org/",
		"citeAs":             "cr:citeAs",
		"column":             "cr:column",
		"conformsTo":         "dct:conformsTo",
		"cr":                 "http://mlcommons.org/croissant/",
		"rai":                "http://mlcommons.org/croissant/RAI/",
		"data":               map[string]any{"@id": "cr:data", "@type": "@json"},
		"dataType":           map[string]any{"@id": "cr:dataType", "@type": "@vocab"},
		"dct":                "http://purl.org/dc/terms/",
		"equivalentProperty": "cr:equivalentProperty",
		"examples":           map[string]any{"@id": "cr:examples", "@type": "@json"},
		"extract":            "cr:extract",
		"field":              "cr:field",
		"fileProperty":       "cr:fileProperty",
		"fileObject":         "cr:fileObject",
		"fileSet":            "cr:fileSet",
		"format":             "cr:format",
		"includes":           "cr:includes",
		"isLiveDataset":      "cr:isLiveDataset",
		"jsonPath":           "cr:jsonPath",
		"key":                "cr:key",
		"md5":                "cr:md5",
		"parentField":        "cr:parentField",
		"path":               "cr:path",
		"recordSet":          "cr:recordSet",
		"references":         "cr:references",
		"regex":              "cr:regex",
		"repeated":           "cr:repeated",
		"replace":            "cr:replace",
		"sc":                 "https://schema.org/",
		"samplingRate":       "cr:samplingRate",
		"separator":          "cr:separator",
		"source":             "cr:source",
		"subField":           "cr:subField",
		"transform":          "cr:transform",
	}
}

// licenseLabel maps the platform license type to a human-readable license note.
// Bespoke commercial terms have no SPDX id, so we use descriptive text plus the
// platform Terms URL as the authoritative source.
func licenseLabel(licenseType string) string {
	switch licenseType {
	case licenseCommercial:
		return "Commercial-use license — see platform Terms of Service"
	case licenseResearch:
		return "Research-only license — see platform Terms of Service"
	case licenseTrainOnly:
		return "AI-training-only license — see platform Terms of Service"
	default:
		return licenseType
	}
}

// BuildCroissant assembles a Croissant 1.0 JSON-LD document for a dataset from
// its metadata, current-version file info, and quality checks. Pure function —
// no I/O — so it is fully unit-testable. baseURL is the public site origin used
// to build URLs (e.g. https://host).
func BuildCroissant(d Dataset, vm VersionMeta, checks []QualityCheck, baseURL string) map[string]any {
	datasetURL := strings.TrimRight(baseURL, "/") + "/datasets/" + d.ID

	description := strings.TrimSpace(d.Description)
	if description == "" {
		description = "AI training dataset on Verdant Oasis（绿洲）."
	}

	doc := map[string]any{
		"@context":    croissantContext(),
		"@type":       "sc:Dataset",
		"conformsTo":  "http://mlcommons.org/croissant/1.0",
		"name":        d.Title,
		"description": description,
		"url":         datasetURL,
		"license":     licenseLabel(d.LicenseType),
		"version":     fmt.Sprintf("%d.0", maxInt(vm.VersionNo, 1)),
		"creator": map[string]any{
			"@type": "Organization",
			"name":  "Verdant Oasis（绿洲）",
			"url":   strings.TrimRight(baseURL, "/"),
		},
	}
	if d.CreatedAt != "" {
		doc["datePublished"] = d.CreatedAt
	}
	// citeAs is recommended by the Croissant spec — a human-readable citation.
	doc["citeAs"] = fmt.Sprintf("Verdant Oasis（绿洲）. %q. %s", d.Title, datasetURL)
	if kw := keywords(d); len(kw) > 0 {
		doc["keywords"] = kw
	}

	// distribution: one FileObject for the current version's file. The bytes are
	// gated (paid), so contentUrl points at the dataset page where access is
	// granted rather than a direct download.
	ct := vm.ContentType
	if ct == "" && vm.ObjectKey != "" {
		ct = contentTypeOf(vm.ObjectKey)
	}
	fileName := "data"
	if vm.ObjectKey != "" {
		fileName = path.Base(vm.ObjectKey)
	}
	if ct != "" || vm.SizeBytes > 0 || vm.ContentSHA256 != "" {
		fo := map[string]any{
			"@type":      "cr:FileObject",
			"@id":        fileName,
			"name":       fileName,
			"contentUrl": datasetURL,
			"description": "Access to the dataset bytes is granted after purchase; " +
				"this entry describes the file for catalog/interoperability purposes.",
		}
		if ct != "" {
			fo["encodingFormat"] = ct
		}
		if vm.SizeBytes > 0 {
			fo["contentSize"] = fmt.Sprintf("%d B", vm.SizeBytes)
		}
		if vm.ContentSHA256 != "" {
			fo["sha256"] = vm.ContentSHA256
		}
		doc["distribution"] = []any{fo}
	}

	if props := qualityProperties(checks); len(props) > 0 {
		doc["additionalProperty"] = props
	}
	applyDatasheet(doc, d.Datasheet)
	return doc
}

// applyDatasheet maps the seller's datasheet onto the Croissant document using
// the Responsible-AI (rai:) namespace where it fits, plus schema.org inLanguage.
// Only populated fields are emitted.
func applyDatasheet(doc map[string]any, ds *Datasheet) {
	if ds == nil {
		return
	}
	set := func(key, val string) {
		if strings.TrimSpace(val) != "" {
			doc[key] = val
		}
	}
	set("rai:dataUseCases", ds.IntendedUses)
	set("rai:dataLimitations", ds.Limitations)
	set("rai:dataCollection", ds.CollectionProcess)
	set("rai:dataPreprocessingProtocol", ds.Preprocessing)
	set("rai:personalSensitiveInformation", ds.EthicalConsiderations)
	if len(ds.Languages) > 0 {
		langs := make([]any, len(ds.Languages))
		for i, l := range ds.Languages {
			langs[i] = l
		}
		doc["inLanguage"] = langs
	}
}

// keywords derives schema.org keywords from the dataset's facets.
func keywords(d Dataset) []string {
	var kw []string
	if d.DataType != "" {
		kw = append(kw, d.DataType)
	}
	if d.Domain != "" {
		kw = append(kw, d.Domain)
	}
	kw = append(kw, "AI training data")
	return kw
}

// qualityProperties surfaces the platform's quality signals as schema.org
// PropertyValue entries — a standards-compliant way to expose our differentiator
// (de-identification proof + statistical authenticity) inside the metadata.
func qualityProperties(checks []QualityCheck) []any {
	var props []any
	add := func(name, value string) {
		props = append(props, map[string]any{"@type": "PropertyValue", "name": name, "value": value})
	}
	for _, c := range checks {
		switch c.Type {
		case "authenticity":
			if applicable, _ := c.Report["applicable"].(bool); applicable {
				if band, ok := c.Report["band"].(string); ok && band != "" {
					add("authenticity_band", band)
				}
				if score, ok := numFromReport(c.Report["score"]); ok {
					add("authenticity_score", fmt.Sprintf("%d", score))
				}
			}
		case "pii_redaction":
			if verified, _ := c.Report["verified"].(bool); verified {
				add("pii_deidentified", "verified-zero-residual")
			}
		}
	}
	if len(props) > 0 {
		add("quality_screened_by", "Verdant Oasis quality engine + PaperGuard")
	}
	return props
}

// numFromReport coerces a JSON number (float64 from json, or int) to int.
func numFromReport(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
