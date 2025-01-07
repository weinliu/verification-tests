#!/bin/bash

set -euo pipefail

ARTIFACT_DIR=${ARTIFACT_DIR:=/tmp/artifacts}
cp -L $KUBECONFIG /tmp/kubeconfig && export CYPRESS_KUBECONFIG_PATH=/tmp/kubeconfig
mkdir -p $ARTIFACT_DIR
SCREENSHOTS_DIR=gui_test_screenshots
source "${SHARED_DIR}/runtime_env"

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

# clone upstream console repo and copy required libs
set -x
git clone -b master --depth=1 --filter=blob:none --sparse https://github.com/openshift/console.git upstream_console
cd upstream_console
git sparse-checkout init --cone && git sparse-checkout set frontend/packages/integration-tests-cypress
cd ../
cp -r ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

# trigger tests
set +x
console_route=$(oc get route console -n openshift-console -o jsonpath='{.spec.host}')
idp_name=$(oc get oauths.config.openshift.io cluster -o jsonpath='{.spec.identityProviders[-1].name}')
export CYPRESS_BASE_URL=https://$console_route
export CYPRESS_LOGIN_IDP=$idp_name
export CYPRESS_LOGIN_USERS=${USERS}
export NO_COLOR=1
ls -ltr
echo "triggering tests"
set -x
if grep -q "rosa" "${SHARED_DIR}/cluster-type"; then
  yarn run test-cypress-console-rosa
elif grep -q "osd" "${SHARED_DIR}/cluster-type"; then
  yarn run test-cypress-console-osd
fi
