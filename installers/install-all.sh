#!/usr/bin/env bash
#
# As more installer scripts are added to the `installers` dir make an entry here
# so install-all does really install everything.

set -euo pipefail

echo "⏳ Installing all configs, scripts, and skills..."

./installers/install-configs.sh
./installers/install-local-bin.sh
./installers/install-skills.sh

# Install Go tools
if [[ -d "azure-capacity-finder" ]]; then
    echo "⏳ Installing azure-capacity-finder..."
    (cd azure-capacity-finder && go install .)
    echo "✅ azure-capacity-finder installed to ~/go/bin"
fi

if [[ -d "clawbox" ]]; then
    echo "⏳ Installing clawbox..."
    (cd clawbox && go install .)
    echo "✅ clawbox installed to ~/go/bin"
fi

echo "✅ All installations completed successfully!"
