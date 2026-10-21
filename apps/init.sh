#!/bin/bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
command -v curl >/dev/null || { echo "Error: curl is required to check the API." >&2; exit 1; }
echo "Building and starting Smart Home services..."
docker compose up --build --wait --wait-timeout 120

# Ask Compose for the published port; never interpret .env as shell code.
address=$(docker compose port app 8080 | head -n 1)
port=${address##*:}
if [[ ! "$port" =~ ^[0-9]+$ ]]; then
  echo "Error: cannot determine the application port." >&2
  exit 1
fi
base_url="http://127.0.0.1:$port"
for ((attempt=1; attempt<=30; attempt++)); do
  if curl --fail --silent --max-time 2 "$base_url/health" >/dev/null; then
    echo "All services are ready. API: $base_url"
    echo "Logs: docker compose logs -f"
    echo "Stop without deleting data: docker compose stop"
    exit 0
  fi
  sleep 1
done
echo "Error: application health check failed at $base_url/health." >&2
exit 1
