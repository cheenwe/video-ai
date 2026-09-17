#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
export BASE_FRONTEND_DIR="${BASE_FRONTEND_DIR:-$PWD/frontend/dist}"
export BASE_DATA_DIR="${BASE_DATA_DIR:-$PWD/data}"
exec ./baseplatform
