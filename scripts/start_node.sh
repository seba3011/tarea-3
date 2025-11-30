#!/bin/bash
if [ -z "$1" ]; then
    exit 1
fi

ID=$1

cd "$(dirname "$0")/.." || exit

LOG_FILE="logs/node${ID}.log"
PID_FILE="logs/node${ID}.pid"

go build -o node${ID}/node node${ID}/main.go

nohup ./node${ID}/node > "$LOG_FILE" 2>&1 &

echo $! > "$PID_FILE"