#!/bin/bash

if [ -z "$1" ]; then
  echo "Uso: ./scripts/stop.sh <nodo_id>"
  exit 1
fi

ID=$1
PID_FILE="logs/node$ID.pid"

if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  echo "🛑 Matando nodo $ID (PID $PID)..."
  kill $PID
  rm "$PID_FILE"
else
  echo "⚠️ No hay PID registrado para nodo $ID"
fi
