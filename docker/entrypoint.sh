#!/bin/bash
set -m
./main &
sleep 5
./machine localhost 8086 http://localhost:8080 1
