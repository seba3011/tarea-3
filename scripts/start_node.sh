#!/bin/bash
if [ -z "$1" ]; then exit 1; fi

ID=$1
BASE_DIR=$(dirname "$0")/..
cd "$BASE_DIR" || exit

LOG_FILE="$BASE_DIR/logs/node${ID}.log"
PID_FILE="$BASE_DIR/logs/node${ID}.pid"
BIN="$BASE_DIR/node${ID}/node"

go build -o "$BIN" "$BASE_DIR/node${ID}/main.go"

nohup "$BIN" > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
