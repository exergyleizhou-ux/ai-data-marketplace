# PaperGuard authenticity sidecar — Python 3.11 (PaperGuard requires >=3.11).
FROM python:3.11-slim

WORKDIR /app

# System libs some PaperGuard/pandas wheels expect at runtime.
RUN apt-get update && apt-get install -y --no-install-recommends \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml ./
RUN pip install --no-cache-dir \
        "fastapi>=0.110" "uvicorn[standard]>=0.29" "paperguard==2.17.0" "pandas>=2.0"

COPY screening.py app.py ./

# Internal network only; never expose publicly.
EXPOSE 8088
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8088/healthz').status==200 else 1)"

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8088"]
