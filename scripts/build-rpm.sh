#!/bin/bash

set -e

REPO_HOME=$(readlink -f $(dirname $(readlink -f "$0"))/../)

VERSION="${1#v}"
RELEASE="${2-1}"
ARCH="${ARCH:-$(uname -m)}"

: "${VERSION:?Missing argument}"

# RPM package version must not have '-'
VERSION=$(echo "$VERSION" | sed 's/-/_/g')

echo "Setting RPM package version to ${VERSION}-${RELEASE}"

mkdir -p rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

cp $REPO_HOME/rpmbuild/topograph.spec $REPO_HOME/rpmbuild/SPECS/topograph.spec

sed -i "s/__VERSION__/$VERSION/g" $REPO_HOME/rpmbuild/SPECS/topograph.spec
sed -i "s/__RELEASE__/${RELEASE}/g" $REPO_HOME/rpmbuild/SPECS/topograph.spec
sed -i "s/__ARCH__/${ARCH}/g" $REPO_HOME/rpmbuild/SPECS/topograph.spec

cp $REPO_HOME/bin/topograph \
  $REPO_HOME/config/topograph-config.yaml \
  $REPO_HOME/config/topograph.service \
  $REPO_HOME/scripts/configure-ssl.sh \
  $REPO_HOME/scripts/create-topology-update-script.sh \
  $REPO_HOME/rpmbuild/SOURCES/

rpmbuild --define "_topdir $REPO_HOME/rpmbuild" -bb --clean $REPO_HOME/rpmbuild/SPECS/topograph.spec

# Optional override for downstream packagers:
#   RPM_OUTPUT_DIR - directory the .rpm is written to (default: ${REPO_HOME}/bin)
RPM_OUTPUT_DIR="${RPM_OUTPUT_DIR:-${REPO_HOME}/bin}"
mkdir -p "${RPM_OUTPUT_DIR}"
RPM_OUTPUT_FILE="${RPM_OUTPUT_DIR}/topograph-$1.${ARCH}.rpm"
mv $REPO_HOME/rpmbuild/RPMS/${ARCH}/topograph-${VERSION}-${RELEASE}.${ARCH}.rpm "${RPM_OUTPUT_FILE}"
echo "Created ${RPM_OUTPUT_FILE}"
