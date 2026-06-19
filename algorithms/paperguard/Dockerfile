# Verdant Oasis Compute-to-Data sandbox algorithm: PaperGuard data-integrity screen.
# Runs under `docker run --network none --read-only`; dataset read-only at /data,
# output at /out/output.bin. Wraps the published `paperguard` package (PyPI) as a
# black box and emits AGGREGATE integrity statistics only — no raw rows (§7.3/§8).
# Only paperguard's light core deps (pydantic/numpy/scipy/pandas/openpyxl) are
# pulled; screen.py imports only the 8 offline tabular detectors, never the
# image/PDF ones, so no opencv/pymupdf is needed. Build & pin by digest exactly
# like algorithms/logreg (see that README).
FROM python:3.11-slim
# Resilient install: retries + long timeout for flaky networks. PIP_INDEX_URL is a
# build-arg so you can point at a faster mirror (e.g. a domestic PyPI mirror)
# without editing this file; the committed default is the canonical PyPI.
ARG PIP_INDEX_URL=https://pypi.org/simple
RUN pip install --no-cache-dir --retries 10 --timeout 300 -i "${PIP_INDEX_URL}" \
    "paperguard==2.17.0" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +
COPY screen.py /app/screen.py
WORKDIR /app
USER 65534:65534
ENTRYPOINT ["python", "/app/screen.py"]
