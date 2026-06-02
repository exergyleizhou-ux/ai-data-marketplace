# Verdant Oasis Compute-to-Data sandbox algorithm: differentially-private stats.
# Runs under `docker run --network none --read-only`; data read-only at /data,
# output at /out. Pure numpy/pandas — tiny audited surface (design §7.3/§8).
# Build & pin by digest exactly like algorithms/logreg (see that README).
FROM python:3.11-slim
RUN pip install --no-cache-dir "numpy>=1.26" "pandas>=2.1" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +
COPY stats.py /app/stats.py
WORKDIR /app
USER 65534:65534
ENTRYPOINT ["python", "/app/stats.py"]
