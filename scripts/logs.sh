#!/bin/bash

if [ -z "$1" ]; then
  echo "Uso: ./scripts/logs.sh <nodo_id>"
  exit 1
fi

ID=$1
LOG_FILE="logs/node$ID.log"

if [ -f "$LOG_FILE" ]; then
  tail -f "$LOG_FILE"
else
  echo "⚠️ Log no encontrado para nodo $ID"
fi
