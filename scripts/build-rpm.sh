#!/bin/bash

set -e

REPO_HOME=$(readlink -f $(dirname $(readlink -f "$0"))/../)

VERSION="${1#v}"
RELEASE="${2-1}"
ARCH=$(uname -m)

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

mv $REPO_HOME/rpmbuild/RPMS/${ARCH}/topograph-${VERSION}-${RELEASE}.${ARCH}.rpm $REPO_HOME/bin/topograph-$1.${ARCH}.rpm
echo "Created $REPO_HOME/bin/topograph-$1.${ARCH}.rpm"
