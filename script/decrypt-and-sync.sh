#!/bin/bash
set -e
cd "$(dirname "$0")/.."
./bin/chatlog decrypt && ./bin/chatlog sync
