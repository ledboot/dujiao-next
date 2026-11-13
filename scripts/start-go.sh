#!/usr/bin/env bash
set -euo pipefail

# 从任意目录启动本地 Go 后端。
PROJECT_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

set -a
source /Users/Gwynn/workspace/github/dujiao-next-local/.env
set +a

# 从源码启动 Go 后端。
SERVER_HOST=127.0.0.1 \
SERVER_PORT=8080 \
SERVER_MODE=debug \
SERVER_DEV_DISABLE_EMAIL_VERIFICATION=true \
DATABASE_DRIVER=sqlite \
DATABASE_DSN=./db/dujiao-dev.db \
REDIS_ENABLED=true \
QUEUE_ENABLED=true \
exec go run ./cmd/server -mode api
