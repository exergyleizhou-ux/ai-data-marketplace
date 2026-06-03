# Verdant Oasis Compute-to-Data sandbox algorithm: federated logistic regression
# (LOCAL trainer). One per dataset runs under `docker run --network none
# --read-only`, dataset mounted read-only at /data, output written to /out.
# Pure numpy/pandas (no sklearn, no pickle) so the audited surface is tiny.
#
# Build & publish (then pin the DIGEST when registering — a mutable :latest tag
# voids the audit, design §4/§7.3):
#   docker build -t <registry>/vo-fed-logreg:1 algorithms/fed-logreg
#   docker push <registry>/vo-fed-logreg:1
#   docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-fed-logreg:1
#   # register with image=<registry>/vo-fed-logreg, image_digest=sha256:...,
#   # runtime=fed-logreg, output_kind=model, trusted=true
FROM python:3.11-slim

RUN pip install --no-cache-dir "numpy>=1.26" "pandas>=2.1" \
    && find /usr/local/lib/python3.11 -name '__pycache__' -type d -prune -exec rm -rf {} +

COPY train.py /app/train.py
WORKDIR /app

# Run unprivileged; rootfs read-only at runtime; only /out (rw) + /tmp (tmpfs) writable.
USER 65534:65534

ENTRYPOINT ["python", "/app/train.py"]
