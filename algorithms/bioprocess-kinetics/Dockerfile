# Verdant Oasis Compute-to-Data sandbox algorithm: bioprocess growth kinetics
# (Logistic + Zwietering Gompertz curve fitting). Runs under `docker run --network
# none --read-only`; dataset read-only at /data, output at /out/output.bin. The
# C2D-portable core of bos-platform's kinetics_engine; numpy/pandas/scipy only (no
# ODE solver). PIP_INDEX_URL build-arg points at a mirror.
FROM python:3.11-slim
ARG PIP_INDEX_URL=https://pypi.org/simple
RUN pip install --no-cache-dir --retries 10 --timeout 300 -i "${PIP_INDEX_URL}" \
    "numpy>=1.26" "pandas>=2.1" "scipy>=1.11" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +
COPY kinetics.py /app/kinetics.py
WORKDIR /app
USER 65534:65534
ENTRYPOINT ["python", "/app/kinetics.py"]
