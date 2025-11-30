#!/bin/bash

if [ -z "$1" ]; then
    echo "Error: Debe especificar el ID del nodo para ver el log."
    exit 1
fi

ID=$1
LOG_FILE="logs/node${ID}.log"

echo "=========================================="
echo " MONITOREANDO LOG DE NODO $ID"
echo "=========================================="

if [ -f "$LOG_FILE" ]; then
    tail -f "$LOG_FILE"
else
    echo "Error: El archivo de log '$LOG_FILE' no existe."
    echo "Asegúrese de que el nodo $ID se haya iniciado con ./scripts/start_node.sh $ID"
fi