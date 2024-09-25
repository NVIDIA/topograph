#!/bin/bash

set -e

REPO_HOME=$(readlink -f $(dirname $(readlink -f "$0"))/../)

rm -rf ${REPO_HOME}/bin 

rm -f deb/topograph/DEBIAN/control

for dir in usr/local etc/topograph lib; do
  rm -rf ${REPO_HOME}/deb/topograph/$dir
done

rm -rf ${REPO_HOME}/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
