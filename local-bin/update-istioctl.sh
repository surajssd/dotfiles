#!/bin/bash



# Determines the operating system.
OS="$(uname)"
if [ "x${OS}" = "xDarwin" ] ; then
  OSEXT="osx"
else
  OSEXT="linux"
fi

# Determines the istioctl version.
if [ "x${ISTIO_VERSION}" = "x" ] ; then
  ISTIO_VERSION=$(curl -L -s https://api.github.com/repos/istio/istio/releases | \
                  grep tag_name | sed "s/ *\"tag_name\": *\"\\(.*\\)\",*/\\1/" | \
                  grep -v -E "(alpha|beta|rc)\.[0-9]$" | sort -t"." -k 1,1 -k 2,2 -k 3,3 -k 4,4 | tail -n 1)
fi

if [ "x${ISTIO_VERSION}" = "x" ] ; then
  printf "Unable to get latest Istio version. Set ISTIO_VERSION env var and re-run. For example: export ISTIO_VERSION=1.0.4"
  exit;
fi

LOCAL_ARCH=$(uname -m)
if [ "${TARGET_ARCH}" ]; then
    LOCAL_ARCH=${TARGET_ARCH}
fi

set -euo pipefail

case "${LOCAL_ARCH}" in
  x86_64)
    ISTIO_ARCH=amd64
    ;;
  armv8*)
    ISTIO_ARCH=arm64
    ;;
  aarch64*)
    ISTIO_ARCH=arm64
    ;;
  armv*)
    ISTIO_ARCH=armv7
    ;;
  amd64|arm64)
    ISTIO_ARCH=${LOCAL_ARCH}
    ;;
  *)
    echo "This system's architecture, ${LOCAL_ARCH}, isn't supported"
    exit 1
    ;;
esac

download_failed () {
  printf "Download failed, please make sure your ISTIO_VERSION is correct and verify the download URL exists!"
  exit 1
}

# Downloads the istioctl binary archive.
tmp=$(mktemp -d /tmp/istioctl.XXXXXX)
NAME="istioctl-${ISTIO_VERSION}"

cd "$tmp" || exit
URL="https://github.com/istio/istio/releases/download/${ISTIO_VERSION}/istioctl-${ISTIO_VERSION}-${OSEXT}.tar.gz"
ARCH_URL="https://github.com/istio/istio/releases/download/${ISTIO_VERSION}/istioctl-${ISTIO_VERSION}-${OSEXT}-${ISTIO_ARCH}.tar.gz"

printf "Downloading %s from %s ... \n" "${NAME}" "${URL}"
if ! curl -fsLO "$URL"
then
  printf "Failed. \n\nTrying with TARGET_ARCH. Downloading %s from %s ...\n" "${NAME}" "$ARCH_URL"
  if ! curl -fsLO "$ARCH_URL"
  then
   download_failed
  else
    filename="istioctl-${ISTIO_VERSION}-${OSEXT}-${ISTIO_ARCH}.tar.gz"
    tar -xzf "${filename}"
  fi
else
  filename="istioctl-${ISTIO_VERSION}-${OSEXT}.tar.gz"
  tar -xzf "${filename}"
fi
printf "%s download complete!\n" "${filename}"

# setup istioctl
cd "$HOME" || exit
mv "${tmp}/istioctl" ".local/bin/istioctl"
chmod +x ".local/bin/istioctl"
rm -r "${tmp}"

# Print message
printf "\n"
printf "Begin the Istio pre-installation verification check by running:\n"
printf "\t istioctl verify-install \n"
printf "\n"
printf "Need more information? Visit https://istio.io/docs/reference/commands/istioctl/ \n"
