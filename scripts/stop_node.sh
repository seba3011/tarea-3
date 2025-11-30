#!/bin/bash
if [ -z "$1" ]; then
    exit 1
fi

ID=$1

case $ID in
  1) PORT=8081 ;;
  2) PORT=8082 ;;
  3) PORT=8083 ;;
  *) exit 1 ;;
esac

PID_FILE="logs/node$ID.pid"

if [ -f "$PID_FILE" ]; then
  kill -9 $(cat "$PID_FILE") 2>/dev/null
  rm -f "$PID_FILE"
fi

kill -9 $(lsof -t -i :$PORT) 2>/dev/null
