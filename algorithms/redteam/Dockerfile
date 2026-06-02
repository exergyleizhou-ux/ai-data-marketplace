# Red-team payload for the §19 adversarial sandbox suite. NOT a real algorithm —
# never register it as approved/trusted. stdlib only (no deps).
FROM python:3.11-slim
COPY attack.py /app/attack.py
WORKDIR /app
USER 65534:65534
ENTRYPOINT ["python", "/app/attack.py"]
