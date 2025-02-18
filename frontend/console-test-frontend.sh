#!/bin/bash

TESTS_TO_RUN=$1

set -euo pipefail

ARTIFACT_DIR=${ARTIFACT_DIR:=/tmp/artifacts}
cp -L $KUBECONFIG /tmp/kubeconfig && export CYPRESS_KUBECONFIG_PATH=/tmp/kubeconfig
mkdir -p $ARTIFACT_DIR
SCREENSHOTS_DIR=gui_test_screenshots

function copyArtifacts {
    yarn merge-reports
    python3 parse-xml.py
    if [ -d "$ARTIFACT_DIR" ] && [ -d "$SCREENSHOTS_DIR" ]; then
        echo "Copying artifacts from $(pwd)..."
        cp console-cypress.xml "$SCREENSHOTS_DIR"
        cp -r "$SCREENSHOTS_DIR" "${ARTIFACT_DIR}/gui_test_screenshots"
    fi
}

## Add IDP for testing
# prepare users
users=""
htpass_file=/tmp/users.htpasswd

for i in $(seq 1 5); do
    username="uiauto-test-${i}"
    password=$(tr </dev/urandom -dc 'a-z0-9' | fold -w 12 | head -n 1 || true)
    users+="${username}:${password},"
    if [ -f "${htpass_file}" ]; then
        htpasswd -B -b ${htpass_file} "${username}" "${password}"
    else
        htpasswd -c -B -b ${htpass_file} "${username}" "${password}"
    fi
done
# remove trailing ',' for case parsing
users=${users%?}

# current generation
gen=$(oc get deployment oauth-openshift -n openshift-authentication -o jsonpath='{.metadata.generation}')

# add users to cluster
oc create secret generic uiauto-htpass-secret --from-file=htpasswd=${htpass_file} -n openshift-config
oc patch oauth cluster --type='json' -p='[{"op": "add", "path": "/spec/identityProviders", "value": [{"type": "HTPasswd", "name": "uiauto-htpasswd-idp", "mappingMethod": "claim", "htpasswd":{"fileData":{"name": "uiauto-htpass-secret"}}}]}]'

## wait for oauth-openshift to rollout
wait_auth=true
expected_replicas=$(oc get deployment oauth-openshift -n openshift-authentication -o jsonpath='{.spec.replicas}')
while $wait_auth; do
    available_replicas=$(oc get deployment oauth-openshift -n openshift-authentication -o jsonpath='{.status.availableReplicas}')
    new_gen=$(oc get deployment oauth-openshift -n openshift-authentication -o jsonpath='{.metadata.generation}')
    if [[ $expected_replicas == "$available_replicas" && $((new_gen)) -gt $((gen)) ]]; then
        wait_auth=false
    else
        sleep 10
    fi
done
echo "authentication operator finished updating"
trap copyArtifacts EXIT

# clone upstream console repo and create soft link
set -x
git clone -b main --depth=1 https://github.com/openshift/console.git upstream_console && cd upstream_console/frontend && yarn install
cd ../../
ln -s ./upstream_console/frontend/packages/integration-tests-cypress upstream

# in frontend dir, install deps and trigger tests
yarn install

# trigger tests
set +x
console_route=$(oc get route console -n openshift-console -o jsonpath='{.spec.host}')
export CYPRESS_BASE_URL=https://$console_route
export CYPRESS_LOGIN_IDP=uiauto-htpasswd-idp
export CYPRESS_LOGIN_USERS=$users
export NO_COLOR=1
ls -ltr
echo "triggering tests"
set -x
if [[ ${TESTS_TO_RUN} == '--spec' ]]; then
    node --max-old-space-size=4096 ./node_modules/.bin/cypress run $@ --browser chrome
elif [[ ${TESTS_TO_RUN} == '--tags' ]]; then
    node --max-old-space-size=4096 ./node_modules/.bin/cypress run --env grepTags=$2 openshift=true --browser chrome
else
    yarn run test-cypress-console-headless
fi
