#!/bin/bash
set -euo pipefail
ARTIFACT_DIR=${ARTIFACT_DIR:=/tmp/artifacts}
mkdir -p $ARTIFACT_DIR
SCREENSHOTS_DIR=gui_test_screenshots
echo $SHARED_DIR
function copyArtifacts {
  yarn merge-reports
  python3 parse-xml.py
  if [ -d "$ARTIFACT_DIR" ] && [ -d "$SCREENSHOTS_DIR" ]; then
    echo "Copying artifacts from $(pwd)..."
    cp console-cypress.xml "$SCREENSHOTS_DIR"
    cp -r "$SCREENSHOTS_DIR" "${ARTIFACT_DIR}/gui_test_screenshots"
  fi
}
trap copyArtifacts EXIT

# clone upstream console repo and create soft link
set -x
git clone -b master --depth=1 https://github.com/openshift/console.git upstream_console && cd upstream_console/frontend && yarn install
cd ../../
ln -s ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

set +x
export CYPRESS_BASE_URL=`cat $SHARED_DIR/hostedcluster_console.url`
export CYPRESS_LOGIN_IDP=kube:admin
export CYPRESS_LOGIN_USERNAME=kubeadmin
export CYPRESS_LOGIN_PASSWORD=`cat $SHARED_DIR/hostedcluster_kubeadmin_password`
export CYPRESS_KUBECONFIG_PATH="${KUBECONFIG}"
ls -ltr
echo "Triggering tests"
set -x
yarn run test-cypress-console-hypershift-guest
