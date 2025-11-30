#!/bin/bash
if [ -z "$1" ]; then
 echo "Error: Debe especificar el ID del nodo a detener."
 echo "Uso: ./scripts/stop_node.sh <1|2|3>"
 exit 1
fi

ID=$1
PID_FILE="logs/node$ID.pid"

echo "=========================================="
echo " DETENIENDO NODO $ID"
echo "=========================================="

if [ -f "$PID_FILE" ]; then
 PID=$(cat "$PID_FILE")
 echo "-> PID encontrado: $PID"
 kill -9 $PID 2>/dev/null 
 sleep 0.5
 rm -f "$PID_FILE"
 echo "-> Archivo PID eliminado."
 echo "-> Nodo $ID TERMINADO."
else
 echo "-> Error: No hay PID registrado en $PID_FILE. El nodo podría no estar corriendo."
fi