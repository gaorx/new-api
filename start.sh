#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_PATH="$ROOT_DIR/dist/new-api"
LOG_DIR="$ROOT_DIR/logs"
mkdir -p "$ROOT_DIR/dist"
mkdir -p "$LOG_DIR"

echo "Building backend binary..."
go build -o "$BIN_PATH" main.go && "$BIN_PATH" --log-dir "$LOG_DIR" "$@"
