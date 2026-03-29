#!/usr/bin/env bash
# Use this script to delete an Azure Resource Group in a loop. This is useful
# when the resource group is stuck in "Deleting" state due to some resources
# that are taking a long time to delete.

set -euo pipefail

AZURE_RESOURCE_GROUP="${1:-${AZURE_RESOURCE_GROUP:-}}"
: "${AZURE_RESOURCE_GROUP:?AZURE_RESOURCE_GROUP must be provided either as first argument or environment variable}"

SUBSCRIPTION_ID=$(az account show --query id --output tsv)

MAX_ATTEMPTS=60 # 60 attempts * 30 seconds = 30 minutes
attempt=0

while true; do
    az group delete \
        -g "${AZURE_RESOURCE_GROUP}" \
        --subscription "${SUBSCRIPTION_ID}" \
        -y --no-wait
    date

    # Sleep if the resource group is still there otherwise exit the loop.
    if az group show -g "${AZURE_RESOURCE_GROUP}" >/dev/null 2>&1; then
        attempt=$((attempt + 1))
        if [ "${attempt}" -ge "${MAX_ATTEMPTS}" ]; then
            echo "❌ Timed out after $((MAX_ATTEMPTS * 30 / 60)) minutes waiting for resource group ${AZURE_RESOURCE_GROUP} to be deleted."
            exit 1
        fi
        echo "⏳ Resource group ${AZURE_RESOURCE_GROUP} still exists. Attempt ${attempt}/${MAX_ATTEMPTS}. Sleeping for 30 seconds..."
        sleep 30
    else
        echo "✅ Resource group ${AZURE_RESOURCE_GROUP} deleted successfully."
        exit 0
    fi
done
