#!/bin/bash

if [ -z "$1" ]; then
  echo "Uso: ./scripts/start.sh <nodo_id>"
  exit 1
fi

ID=$1
cd node$ID || exit

echo "Iniciando nodo $ID..."
nohup go run main.go > "../logs/node$ID.log" 2>&1 & echo $! > "../logs/node$ID.pid"
