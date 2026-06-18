# Verdant Oasis Compute-to-Data sandbox algorithm: Gaussian Naive Bayes.
#
# Runs inside the L1 sandbox under `docker run --network none --read-only`,
# dataset mounted read-only at /data, output written to /out, optional params at
# /params.json. PURE Python stdlib — no numpy/pandas/sklearn — so the audited
# surface is a single small file and the image needs no pip install at all.
#
# Build & publish (pin the DIGEST when registering — a mutable :latest tag voids
# the audit, design §4/§7.3):
#   docker build -t <registry>/vo-gnb:1 algorithms/gnb
#   docker push <registry>/vo-gnb:1
#   docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-gnb:1
#   # register image=<registry>/vo-gnb, image_digest=sha256:...
FROM python:3.11-slim

COPY train.py /app/train.py
WORKDIR /app

USER 65534:65534

ENTRYPOINT ["python", "/app/train.py"]
