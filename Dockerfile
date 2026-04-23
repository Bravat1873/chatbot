FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_INDEX_URL="https://mirrors.tencent.com/pypi/simple/" \
    PIP_TRUSTED_HOST="mirrors.tencent.com"

WORKDIR /app

RUN sed -i 's|http://deb.debian.org/debian|https://mirrors.tencent.com/debian|g; s|http://deb.debian.org/debian-security|https://mirrors.tencent.com/debian-security|g' /etc/apt/sources.list.d/debian.sources

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates libportaudio2 \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml README.md ./
COPY src ./src
COPY scripts ./scripts

RUN python -m pip install --no-cache-dir --upgrade pip \
    && python -m pip install --no-cache-dir .

EXPOSE 8000

CMD ["python", "-m", "uvicorn", "src.gateway.app:app", "--host", "0.0.0.0", "--port", "8000"]
