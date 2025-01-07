# OpenShift Console Tests
openshift web tests relies on upstream [openshift/console](https://github.com/openshift/console/tree/master) which provides fundamental configurations, views that we can reuse in openshift-tests-private web tests

## Prerequisite
1. [node.js](https://nodejs.org/) >= 18 & [yarn](https://yarnpkg.com/en/docs/install) >= 1.20


## Local Development
1. only clone `frontend/packages/integration-tests-cypress` subdirectory of [openshift/console](https://github.com/openshift/console/) repo
  - git clone -b master --depth=1 --filter=blob:none --sparse https://github.com/openshift/console.git upstream_console
  - cd upstream_console
  - git sparse-checkout init --cone && git sparse-checkout set frontend/packages/integration-tests-cypress
2. change dir to `frontend` of openshift-tests-private repo and create a hard copy of upstream_console
  - cd frontend
  - cp -r /path/to/upstream_console/frontend/packages/integration-tests-cypress upstream
3. install required dependencies(in `frontend` dir)
```bash
$ yarn install
$ ls -ltr
node_modules/     -> dependencies will be installed at runtime here
```

### Export necessary variables
in order to run Cypress tests, we need to export some environment variables that Cypress can read then pass down to our tests, currently we have following environment variables defined and used.
```bash
export CYPRESS_BASE_URL=https://<console_route_spec_host>
export CYPRESS_LOGIN_IDP=flexy-htpasswd-provider
**[Note] Use `flexy-htpasswd-provider` above when running tests on flexy installed clusters and using any user other than kubeadmin. Use `kube:admin` when running tests as kubeadmin
export CYPRESS_LOGIN_USERS=USER1:Password1,USER2:Password2,USER3:Password3
export CYPRESS_KUBECONFIG_PATH=/path/to/kubeconfig
```
### Start Cypress
we can either open Cypress GUI(open) or run Cypress in headless mode(run)
```bash
npx cypress open
npx cypress run --env grep="Smoke"

```
