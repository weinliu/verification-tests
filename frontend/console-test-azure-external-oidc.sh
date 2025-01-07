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

# clone upstream console repo and copy required libs
set -x
git clone -b master --depth=1 --filter=blob:none --sparse https://github.com/openshift/console.git upstream_console
cd upstream_console
git sparse-checkout init --cone && git sparse-checkout set frontend/packages/integration-tests-cypress
cd ../
cp -r ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

set +x
export KUBECONFIG=${SHARED_DIR}/kubeconfig
console_route=$(oc get route console -n openshift-console -o jsonpath='{.spec.host}')
export CYPRESS_BASE_URL=https://$console_route
export CYPRESS_LOGIN_USERS=${USER}
export CYPRESS_KUBECONFIG_PATH=${SHARED_DIR}/kubeconfig
export NO_COLOR=1
#environment variables required by external oidc scenario:
export CYPRESS_LOGIN_USER_EMAIL=${USER_EMAIL}
export CYPRESS_LOGIN_USER=${USER}
export CYPRESS_LOGIN_PASSWD=${PASSWORD}

ls -ltr
echo "Triggering tests"
set -x
sed -i 's/trashAssetsBeforeRuns: true/trashAssetsBeforeRuns: false/g' cypress.config.ts
node --max-old-space-size=4096 ./node_modules/.bin/cypress run --env --spec=tests/external-oidc/externaloidc-ui.cy.ts || true
rm -rf ~/.kube/cache/oc/*
nohup oc login --exec-plugin=oc-oidc --issuer-url=$issuer_url --client-id=$cli_client_id --extra-scopes=email,offline_access --callback-port=8080 > /tmp/out.file 2>&1 &
node --max-old-space-size=4096 ./node_modules/.bin/cypress run --env --spec=tests/external-oidc/externaloidc-cli.cy.ts || true
cat /tmp/out.file
if grep -q 'Logged in' /tmp/out.file;then
  ls ~/.kube/cache/oc/ > $SHARED_DIR/oc-oidc-token-filename
  cp ~/.kube/cache/oc/* $SHARED_DIR/oc-oidc-token
  echo "Have logged in successfully.";exit 0;
else
  echo "Login failed.";exit 1;
fi
