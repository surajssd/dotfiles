#!/usr/bin/env bash
#
# As more installer scripts are added to the `installers` dir make an entry here
# so install-all does really install everything.

set -euo pipefail

echo "⏳ Installing all configs, scripts, and skills..."

./installers/install-configs.sh
./installers/install-local-bin.sh
./installers/install-skills.sh

echo "✅ All installations completed successfully!"
