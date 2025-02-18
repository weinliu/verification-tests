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
git clone -b main --depth=1 https://github.com/openshift/console.git upstream_console && cd upstream_console/frontend && yarn install
cd ../../
ln -s ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

set +x
source "${SHARED_DIR}/runtime_env"
cp -L ${SHARED_DIR}/kubeconfig /tmp/kubeconfig
export KUBECONFIG=/tmp/kubeconfig
console_route=$(oc get route console -n openshift-console -o jsonpath='{.spec.host}')
idp_name=$(oc get oauths.config.openshift.io cluster -o jsonpath='{.spec.identityProviders[-1].name}')
export CYPRESS_BASE_URL=https://$console_route
export CYPRESS_LOGIN_IDP=$idp_name
export CYPRESS_LOGIN_USERS=${USERS}
export CYPRESS_KUBECONFIG_PATH="/tmp/kubeconfig"
export NO_COLOR=1
ls -ltr
echo "Triggering tests"
set -x
yarn run test-cypress-console-hypershift-guest
