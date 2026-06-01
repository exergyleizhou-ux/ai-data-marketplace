# PaperGuard 真实性筛查 Sidecar / Authenticity Sidecar

把 [PaperGuard](https://github.com/exergyleizhou-ux/PaperGuard) 的**数值表格取证检测器**封装成一个内网 HTTP 服务，供绿洲后端质检 worker 调用，为表格数据集产出 **0–100 数据真实性分 + clean/review/suspect 三档**。

> **信号，非结论。** 每条发现都带方法引用与「无辜解释」，沿用 PaperGuard 的认知立场——邀请复核，绝不判定造假。与《用户服务协议》§6/§12 呼应。

## 它做什么 / 不做什么

- ✅ 只运行 PaperGuard 的**数值表格检测器子集 `A1 A2 A3 A5 A6 A7 D1 D2`**（末位数字、Benford、GRIM 族、列间关系等），与 PaperGuard CLI 的 `_run_detectors_on_file` 表格分支**完全一致**。
- ❌ 不运行论文专用能力（图像取证、全文/tortured-phrase、引用图谱、撤稿/PubPeer 交叉核验、EXIF/rsid）——与数据交易无关。

## 接口契约

与 Go 客户端 `backend/internal/modules/dataset/sidecar.go` 对齐。

```
GET  /healthz   -> {"status":"ok","paperguard_version":"2.17.0"}

POST /v1/screen
     body: 原始 CSV/TSV 字节；Header: Content-Type: text/csv
     200 -> {
       "schema_version": "1.0",
       "engine": {"paperguard_version": "...", "detectors_run": 8},
       "summary": {"authenticity_score": 0-100, "band": "clean|review|suspect",
                   "n_findings": N, "columns_screened": N, "rows": N, "truncated": false},
       "findings": [{"detector","detector_name","sheet","column","reference","summary",
                     "severity","p_value","p_value_adjusted","statistic","test_name",
                     "significant","innocent_explanations"}],
       "errors": []
     }
     400 -> 空 body
     500 -> {"error": "..."}   # Go worker 据此降级到内置 Go 基线，绝不阻断上架
```

**评分口径与 Go 基线一致**（`backend quality/authenticity.go`）：起始 100，对每条 FDR 显著（或 CONCERN+）发现按严重度扣分（info 0 / low 4 / medium 10 / high 20），`band` 阈值 ≥85 clean、≥60 review、否则 suspect。无论由 sidecar 还是 Go 基线产出，分数含义相同。

## 与后端的集成（降级优先）

后端 worker 始终先算 Go 基线，再在配置了 sidecar 且为表格数据时用 sidecar 结果覆盖；sidecar 不可用/报错则保留 Go 基线——sidecar **永不在关键路径上**。

启用：给后端设环境变量

```
QUALITY_SIDECAR_URL=http://paperguard-sidecar:8088
```

未设置时后端只用 Go 基线，零影响。

## 运行与验证

```bash
# 构建 + 运行（需 Python 3.11；PaperGuard 要求 >=3.11）
docker build -t verdant-paperguard-sidecar .
docker run --rm -p 8088:8088 verdant-paperguard-sidecar

# 健康检查
curl localhost:8088/healthz

# 筛查一个伪造末位数字的列（应进入 review/suspect）
printf 'value\n%s' "$(seq 0 5 1495 | tr '\n' '\n')" \
  | curl -s -XPOST --data-binary @- -H 'Content-Type: text/csv' localhost:8088/v1/screen | jq .summary
```

### 测试

- `tests/test_screening.py` —— 纯逻辑（评分/分桶/映射），**无需 PaperGuard/FastAPI**，stdlib `unittest`，任何环境可跑：
  ```bash
  python tests/test_screening.py
  ```
- `tests/test_contract.py` —— 端到端（需 PaperGuard + FastAPI），未安装则自动 skip：
  ```bash
  pip install -e ".[dev]" && pytest
  ```

## 设计与路线

详见 `docs/设计文档-质量筛查与数据真实性引擎(PaperGuard接入).md`。本 sidecar 是 Go 原生基线之上的**深度增强**：基线常驻保底，sidecar 在则提供 GRIM/GRIMMER/SPRITE 等更深检测器，二者评分口径统一。
