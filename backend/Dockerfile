# IMPORTANT: Must use Debian-based image to solve `grpcio` dependency issue: ImportError: cannot import name 'cygrpc' from 'grpc._cython'
# https://github.com/bazelbuild/rules_docker/issues/1561#issuecomment-666483944

FROM python:3.14-slim-bookworm AS builder
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

WORKDIR /app

# Use the system Python across both stages
COPY pyproject.toml uv.lock ./
RUN uv sync --no-dev --locked

FROM python:3.14-slim-bookworm

COPY --from=builder --chown=root /app /app
ENV PATH="/app/.venv/bin:$PATH"

WORKDIR /app

COPY . .

RUN adduser --disabled-password backend && \
    chown -R backend:backend /app
USER backend

EXPOSE 50051

CMD ["python", "main.py"]