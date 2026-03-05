#!/usr/bin/env bash
set -euo pipefail

if command -v sealos >/dev/null 2>&1; then
  sealos version
  exit 0
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

cd "${tmp_dir}"
until curl -sSfLo sealos.tar.gz "https://github.com/labring/sealos/releases/download/v5.1.0-beta3/sealos_5.1.0-beta3_linux_amd64.tar.gz"; do
  sleep 3
done

tar -zxf sealos.tar.gz sealos
chmod +x sealos
mv sealos /usr/bin/sealos
sealos version
