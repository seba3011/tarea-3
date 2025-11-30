#!/bin/bash
if [ -z "$1" ]; then
    echo "Error: Debe especificar el ID del nodo."
    exit 1
fi

ID=$1

cd "$(dirname "$0")/.." || exit

LOG_FILE="logs/node${ID}.log"
PID_FILE="logs/node${ID}.pid"

echo "=========================================="
echo " INICIANDO NODO $ID"
echo "=========================================="
echo "-> Log: $LOG_FILE"
echo "-> PID: $PID_FILE"

nohup go run node"$ID"/main.go > "$LOG_FILE" 2>&1 &

echo $! > "$PID_FILE"

echo "-> Nodo $ID iniciado con PID: $(cat "$PID_FILE")"