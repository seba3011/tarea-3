#!/bin/bash
if [ -z "$1" ]; then
    exit 1
fi

ID=$1
LOG_FILE="logs/node${ID}.log"

if [ -f "$LOG_FILE" ]; then
    tail -f "$LOG_FILE"
fi
