#!/bin/sh
set -eu
cd "$(dirname "$0")"

podman compose up -d
until podman exec vpn_master_db pg_isready -U master_user -d vpn_core >/dev/null 2>&1; do
  sleep 1
done

cd ..
export DATABASE_URL="postgres://master_user:$(grep -oP '(?<=POSTGRES_PASSWORD=).*' db/.env)@localhost:5432/vpn_core"
cargo sqlx prepare
