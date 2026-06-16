# TEMPLATE — Verdant Oasis Compute-to-Data sandbox algorithm.
# Copy this dir, rename, edit train.py's compute(). Build & register digest-pinned
# (a mutable :latest tag voids the audit, design §4/§7.3):
#   docker build -t <registry>/vo-myalgo:1 algorithms/myalgo
#   docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-myalgo:1
FROM python:3.11-slim
RUN pip install --no-cache-dir "numpy>=1.26" "pandas>=2.1" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +
COPY train.py /app/train.py
WORKDIR /app
USER 65534:65534
ENTRYPOINT ["python", "/app/train.py"]
