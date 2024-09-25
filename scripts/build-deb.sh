#!/bin/bash

set -e

REPO_HOME=$(readlink -f $(dirname $(readlink -f "$0"))/../)

VERSION="${1#v}"
RELEASE="${2//_/-}"
ARCH=$(dpkg --print-architecture)

: "${VERSION:?Missing argument}"

# Debian package version must start with a digit
if [[ ! $VERSION =~ ^[0-9] ]]; then
  VERSION="0.$VERSION"
fi

echo "Setting Debian package version to $VERSION"

cp $REPO_HOME/deb/control $REPO_HOME/deb/topograph/DEBIAN/control

sed -i "s/__VERSION__/$VERSION/g" $REPO_HOME/deb/topograph/DEBIAN/control
sed -i "s/__ARCH__/$ARCH/g" $REPO_HOME/deb/topograph/DEBIAN/control
if [[ -n "${RELEASE}" ]]; then
    sed -i "s/__RELEASE__/-${RELEASE}/g" $REPO_HOME/deb/topograph/DEBIAN/control
else
    sed -i "s/__RELEASE__//g" $REPO_HOME/deb/topograph/DEBIAN/control
fi

# Copy binary
mkdir -p $REPO_HOME/deb/topograph/usr/local/bin
cp $REPO_HOME/bin/topograph $REPO_HOME/deb/topograph/usr/local/bin/

# Copy config file and helper scripts
mkdir -p $REPO_HOME/deb/topograph/etc/topograph
cp $REPO_HOME/config/topograph-config.yaml \
  $REPO_HOME/scripts/configure-ssl.sh \
  $REPO_HOME/scripts/create-topology-update-script.sh \
  $REPO_HOME/deb/topograph/etc/topograph

# Copy service file
mkdir -p $REPO_HOME/deb/topograph/lib/systemd/system
cp $REPO_HOME/config/topograph.service \
  $REPO_HOME/deb/topograph/lib/systemd/system

# Build Debian package
chmod -R 0755 $REPO_HOME/deb/topograph/DEBIAN

dpkg-deb --build $REPO_HOME/deb/topograph
DEB_FILE_PATH="${REPO_HOME}/bin/topograph-$1.${ARCH}.deb"

# Create package parent directory
mkdir -p $(dirname ${DEB_FILE_PATH})
mv $REPO_HOME/deb/topograph.deb ${DEB_FILE_PATH}
echo "Created ${DEB_FILE_PATH}"
