# Verdant Oasis Compute-to-Data sandbox algorithm: k-means clustering.
#
# Runs inside the L1 sandbox under `docker run --network none --read-only`,
# dataset mounted read-only at /data, output written to /out. Pure numpy/pandas
# (no sklearn, no pickle) so the audited surface is tiny.
#
# Build & publish (then pin the DIGEST when registering — a mutable :latest tag
# voids the audit, design §4/§7.3):
#   docker build -t <registry>/vo-kmeans:1 algorithms/kmeans
#   docker push <registry>/vo-kmeans:1
#   docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-kmeans:1
#   # register image=<registry>/vo-kmeans, image_digest=sha256:...
FROM python:3.11-slim

RUN pip install --no-cache-dir "numpy>=1.26" "pandas>=2.1" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +

COPY train.py /app/train.py
WORKDIR /app

USER 65534:65534

ENTRYPOINT ["python", "/app/train.py"]
