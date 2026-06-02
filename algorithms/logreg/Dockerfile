# Verdant Oasis Compute-to-Data sandbox algorithm: logistic regression.
#
# Runs inside the L1 sandbox under `docker run --network none --read-only`,
# with the dataset mounted read-only at /data and output written to /out.
# Pure numpy/pandas (no sklearn, no pickle) so the audited surface is tiny.
#
# Build & publish (then pin the DIGEST when registering the algorithm — a
# mutable :latest tag voids the audit, design §4/§7.3):
#   docker build -t <registry>/vo-logreg:1 algorithms/logreg
#   docker push <registry>/vo-logreg:1
#   docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-logreg:1
#   # register the algorithm with image=<registry>/vo-logreg and
#   # image_digest=sha256:... (the digest from above)
FROM python:3.11-slim

# Pin the only two deps; no build toolchain, no extras.
RUN pip install --no-cache-dir "numpy>=1.26" "pandas>=2.1" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +

COPY train.py /app/train.py
WORKDIR /app

# Run unprivileged. The rootfs is read-only at runtime; the algorithm only
# writes to /out (mounted rw) and /tmp (tmpfs).
USER 65534:65534

ENTRYPOINT ["python", "/app/train.py"]
