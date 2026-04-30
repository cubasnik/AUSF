#!/usr/bin/env bash
# .devcontainer/post-create.sh
# Runs once after the container is created.
# Warms up build caches so the first `make` is fast.
set -euo pipefail

WORKSPACE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORKSPACE_DIR"

echo "==> Toolchain versions"
go version
java --version
mvn --version
python3 --version
cmake --version | head -1
docker compose version

echo ""
echo "==> Go: compile all packages (warms module cache)"
cd "$WORKSPACE_DIR/microservices"
go build ./...
go vet ./...
echo "    Go build: OK"

echo ""
echo "==> Java: download dependencies and compile (skipping tests for speed)"
cd "$WORKSPACE_DIR/control-plane"
mvn --batch-mode --no-transfer-progress dependency:go-offline -q
mvn --batch-mode --no-transfer-progress compile -q
echo "    Maven compile: OK"

echo ""
echo "==> C++: configure CMake build"
cd "$WORKSPACE_DIR"
cmake -S networking -B networking/build -DCMAKE_BUILD_TYPE=Release -G Ninja 2>/dev/null \
  || cmake -S networking -B networking/build -DCMAKE_BUILD_TYPE=Release
cmake --build networking/build
echo "    C++ build: OK"

echo ""
echo "============================================"
echo " Dev container ready."
echo " Quick-start:"
echo "   make compose-up     # start all services via Docker Compose"
echo "   make microservices-test  # run Go unit tests"
echo "   make control-plane-test  # run Java unit tests"
echo "============================================"
