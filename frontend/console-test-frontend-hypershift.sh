#!/bin/bash
set -euo pipefail
ARTIFACT_DIR=${ARTIFACT_DIR:=/tmp/artifacts}
mkdir -p $ARTIFACT_DIR
SCREENSHOTS_DIR=gui_test_screenshots
echo $SHARED_DIR
function copyArtifacts {
  if [ -d "$ARTIFACT_DIR" ] && [ -d "$SCREENSHOTS_DIR" ]; then
    echo "Copying artifacts from $(pwd)..."
    cp console-cypress.xml "$SCREENSHOTS_DIR"
    cp -r "$SCREENSHOTS_DIR" "${ARTIFACT_DIR}/gui_test_screenshots"
  fi
}
trap copyArtifacts EXIT

# clone upstream console repo and create soft link
set -x
git clone -b master https://github.com/openshift/console.git upstream_console && cd upstream_console/frontend && yarn install
cd ../../
ln -s ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

set +x
export BRIDGE_BASE_ADDRESS=`cat $SHARED_DIR/hostedcluster_console.url`
export LOGIN_IDP=kube:admin
export LOGIN_USERNAME=kubeadmin
export LOGIN_PASSWORD=`cat $SHARED_DIR/hostedcluster_kubeadmin_password`
export KUBECONFIG_PATH="${KUBECONFIG}"
ls -ltr
echo "Triggering tests"
set -x
yarn run test-cypress-console-hypershift-guest || yarn merge-reports
python3 parse-xml.py
