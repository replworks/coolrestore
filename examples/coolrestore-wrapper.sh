#!/usr/bin/env bash
set -euo pipefail

environment_file="${COOLRESTORE_ENV_FILE:-/etc/coolrestore/env}"
if [[ ! -r "$environment_file" ]]; then
  echo "coolrestore credential environment file is not readable: $environment_file" >&2
  echo "copy examples/coolrestore.env.example to a protected environment file and set COOLRESTORE_ENV_FILE if needed" >&2
  exit 1
fi

set -a
# The environment file must be administrator-controlled and owner-readable.
# shellcheck disable=SC1090
source "$environment_file"
set +a

exec /usr/local/bin/coolrestore "$@"
