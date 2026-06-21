package main

import (
	"fmt"
	"regexp"
	"strings"
)

// sourceFormat describes how a fetched dataset must be turned into a standard
// comma-delimited CSV before it goes through the platform's quality pipeline.
type sourceFormat string

const (
	formatCSV        sourceFormat = "csv"        // already comma-delimited — pass through
	formatSemicolon  sourceFormat = "semicolon"  // ';'-delimited (and possibly quoted) — e.g. UCI wine-quality
	formatWhitespace sourceFormat = "whitespace" // tab/space-delimited — e.g. UCI seeds, ecoli
)

// seedItem is one public research dataset the platform itself publishes into the
// demo marketplace. Every field is real: the file comes from the cited source,
// the verification is produced by the platform's own quality library, and the
// certificate is the deterministic VO- code over the content fingerprint.
type seedItem struct {
	Key         string
	TitleZH     string
	TitleEN     string
	Domain      string // research vertical, shown as the dataset's domain tag
	DataType    string // always "structured" (these are tabular CSVs)
	LicenseType string // datasets CHECK enum: commercial | research | train_only
	LicenseNote string // human-readable distribution terms (shown in the description)
	SourceURL   string
	Citation    string
	DescZH      string
	DescEN      string
	PriceCents  int64
	Format      sourceFormat
	Header      string // documented column names to prepend when the source file is headerless ("" = already has a header)
}

// prependHeader puts a documented column header in front of a headerless dataset
// so the quality report names real columns instead of using the first data row.
// An empty header leaves the bytes unchanged.
func prependHeader(csv []byte, header string) []byte {
	if header == "" {
		return csv
	}
	return append([]byte(header+"\n"), csv...)
}

var wsRun = regexp.MustCompile(`[ \t]+`)

// normalizeToCSV converts a fetched payload into standard comma-delimited CSV so
// the in-process quality screener (which understands comma/tab) can read every
// column. It is a pure function so it is unit-tested independently of the network.
func normalizeToCSV(raw []byte, format sourceFormat) ([]byte, error) {
	switch format {
	case formatCSV:
		return raw, nil
	case formatSemicolon:
		s := strings.ReplaceAll(string(raw), `"`, "")
		s = strings.ReplaceAll(s, ";", ",")
		return []byte(s), nil
	case formatWhitespace:
		var b strings.Builder
		for _, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue // drop blank/trailing lines; a single final \n is re-added per row
			}
			b.WriteString(wsRun.ReplaceAllString(trimmed, ","))
			b.WriteString("\n")
		}
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("unknown source format %q", format)
	}
}

// seedDatasets is the curated set of real, public research datasets spanning
// eight scientific verticals — the showcase supply for the verified data
// marketplace's research beachhead. All are from the UCI Machine Learning
// Repository (https://archive.ics.uci.edu), distributed under CC BY 4.0, and
// each carries its originating academic citation.
var seedDatasets = []seedItem{
	{
		Key:         "iris",
		TitleZH:     "鸢尾花形态测量数据集（UCI Iris）",
		TitleEN:     "Iris Flower Morphometrics (UCI Iris)",
		Domain:      "植物学 · Botany",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/iris/iris.data",
		Citation:    "Fisher, R.A. (1936). The use of multiple measurements in taxonomic problems. Annals of Eugenics 7(2):179-188.",
		DescZH:      "150 条鸢尾花样本的萼片/花瓣长宽测量，植物分类学经典数据集,常用于形态学与监督分类研究。",
		DescEN:      "150 iris samples with sepal/petal length & width — the classic taxonomy dataset for morphometric and supervised-classification research.",
		PriceCents:  9900,
		Format:      formatCSV,
		Header:      "sepal_length,sepal_width,petal_length,petal_width,species",
	},
	{
		Key:         "wine-quality-red",
		TitleZH:     "红葡萄酒理化指标与品质（UCI Wine Quality）",
		TitleEN:     "Red Wine Physicochemistry & Quality (UCI Wine Quality)",
		Domain:      "食品化学 · Food chemistry",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/wine-quality/winequality-red.csv",
		Citation:    "Cortez, P. et al. (2009). Modeling wine preferences by data mining from physicochemical properties. Decision Support Systems 47(4):547-553.",
		DescZH:      "1,599 款红葡萄酒的 11 项理化指标(酸度、糖、硫、酒精等)与感官评分,食品科学与回归建模常用。",
		DescEN:      "1,599 red wines with 11 physicochemical measures (acidity, sugar, sulfur, alcohol…) and a sensory score — a staple of food science and regression modeling.",
		PriceCents:  19900,
		Format:      formatSemicolon,
	},
	{
		Key:         "glass",
		TitleZH:     "玻璃成分鉴定数据集（UCI Glass）",
		TitleEN:     "Glass Identification (UCI Glass)",
		Domain:      "材料科学 · Materials science",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/glass/glass.data",
		Citation:    "German, B. (1987). Glass Identification Database. Central Research Establishment, Home Office Forensic Science Service.",
		DescZH:      "214 个玻璃样本的折射率与 8 种氧化物含量,材料科学与法医鉴定的成分分析数据集。",
		DescEN:      "214 glass samples with refractive index and 8 oxide concentrations — a compositional-analysis dataset for materials science and forensics.",
		PriceCents:  12900,
		Format:      formatCSV,
		Header:      "id,refractive_index,Na,Mg,Al,Si,K,Ca,Ba,Fe,glass_type",
	},
	{
		Key:         "breast-cancer-wdbc",
		TitleZH:     "威斯康星乳腺癌诊断数据集（UCI WDBC）",
		TitleEN:     "Breast Cancer Wisconsin Diagnostic (UCI WDBC)",
		Domain:      "肿瘤医学 · Oncology",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/breast-cancer-wisconsin/wdbc.data",
		Citation:    "Wolberg, W., Mangasarian, O., Street, N., Street, W. (1995). Breast Cancer Wisconsin (Diagnostic).",
		DescZH:      "569 例乳腺细针穿刺影像的 30 项细胞核形态特征及良/恶性诊断,肿瘤医学与诊断建模基准。",
		DescEN:      "569 fine-needle-aspirate cases with 30 nuclear-morphology features and a benign/malignant diagnosis — a benchmark for oncology and diagnostic modeling.",
		PriceCents:  39900,
		Format:      formatCSV,
		Header:      "id,diagnosis,radius_mean,texture_mean,perimeter_mean,area_mean,smoothness_mean,compactness_mean,concavity_mean,concave_points_mean,symmetry_mean,fractal_dimension_mean,radius_se,texture_se,perimeter_se,area_se,smoothness_se,compactness_se,concavity_se,concave_points_se,symmetry_se,fractal_dimension_se,radius_worst,texture_worst,perimeter_worst,area_worst,smoothness_worst,compactness_worst,concavity_worst,concave_points_worst,symmetry_worst,fractal_dimension_worst",
	},
	{
		Key:         "abalone",
		TitleZH:     "鲍鱼年龄与形态数据集（UCI Abalone）",
		TitleEN:     "Abalone Age & Morphometrics (UCI Abalone)",
		Domain:      "海洋生物学 · Marine biology",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/abalone/abalone.data",
		Citation:    "Nash, W.J. et al. (1994). The Population Biology of Abalone in Tasmania. Sea Fisheries Division, Technical Report 48.",
		DescZH:      "4,177 个鲍鱼样本的尺寸/重量与环数(年龄),渔业资源与海洋生物种群研究数据集。",
		DescEN:      "4,177 abalone with size/weight measures and ring count (age) — a dataset for fisheries and marine-population biology.",
		PriceCents:  29900,
		Format:      formatCSV,
		Header:      "sex,length,diameter,height,whole_weight,shucked_weight,viscera_weight,shell_weight,rings",
	},
	{
		Key:         "seeds",
		TitleZH:     "小麦籽粒几何特征数据集（UCI Seeds）",
		TitleEN:     "Wheat Kernel Geometry (UCI Seeds)",
		Domain:      "农学 · Agronomy",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/00236/seeds_dataset.txt",
		Citation:    "Charytanowicz, M. et al. (2010). Complete Gradient Clustering Algorithm for Features Analysis of X-ray Images. Information Technologies in Biomedicine.",
		DescZH:      "210 个三种小麦品种籽粒的 7 项 X 光几何测量,农学育种与作物表型分析数据集。",
		DescEN:      "210 wheat kernels (3 varieties) with 7 X-ray geometric measures — a dataset for agronomy breeding and crop phenotyping.",
		PriceCents:  14900,
		Format:      formatWhitespace,
		Header:      "area,perimeter,compactness,kernel_length,kernel_width,asymmetry_coeff,kernel_groove_length,variety",
	},
	{
		Key:         "forest-fires",
		TitleZH:     "森林火灾气象数据集（UCI Forest Fires）",
		TitleEN:     "Forest Fires Meteorology (UCI Forest Fires)",
		Domain:      "环境科学 · Environmental science",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/forest-fires/forestfires.csv",
		Citation:    "Cortez, P., Morais, A. (2007). A Data Mining Approach to Predict Forest Fires using Meteorological Data. EPIA 2007.",
		DescZH:      "517 起葡萄牙东北部森林火灾的气象与 FWI 指数及过火面积,环境科学与灾害建模数据集。",
		DescEN:      "517 forest-fire records from NE Portugal with meteorological/FWI indices and burned area — a dataset for environmental science and hazard modeling.",
		PriceCents:  24900,
		Format:      formatCSV,
	},
	{
		Key:         "ecoli",
		TitleZH:     "大肠杆菌蛋白定位数据集（UCI Ecoli）",
		TitleEN:     "E. coli Protein Localization (UCI Ecoli)",
		Domain:      "细胞生物学 · Cell biology",
		DataType:    "structured",
		LicenseType: "research",
		LicenseNote: "CC BY 4.0 · UCI ML Repository",
		SourceURL:   "https://archive.ics.uci.edu/ml/machine-learning-databases/ecoli/ecoli.data",
		Citation:    "Nakai, K., Horton, P. (1996). A Probabilistic Classification System for Predicting the Cellular Localization Sites of Proteins. ISMB 1996.",
		DescZH:      "336 个大肠杆菌蛋白的 7 项序列衍生特征及胞内定位类别,细胞生物学与蛋白组学数据集。",
		DescEN:      "336 E. coli proteins with 7 sequence-derived features and localization-site labels — a dataset for cell biology and proteomics.",
		PriceCents:  17900,
		Format:      formatWhitespace,
		Header:      "sequence_name,mcg,gvh,lip,chg,aac,alm1,alm2,localization_site",
	},
}
