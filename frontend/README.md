# OpenShift Console Tests
openshift web tests relies on upstream [openshift/console](https://github.com/openshift/console/tree/main) which provides fundamental configurations, views that we can reuse in openshift-tests-private web tests

## Prerequisite
1. [node.js](https://nodejs.org/) >= 18 & [yarn](https://yarnpkg.com/en/docs/install) >= 1.20
2. upstream [openshift/console](https://github.com/openshift/console/tree/main) should be cloned locally
3. upstream openshift/console dependencies need to be installed, for example we cloned openshift/console repo and save it to ~/reops
   - cd ~/reops/console/frontend
   - yarn install
4. link openshift/console in `frontend` folder and rename it as `upstream`
   - make sure you are in `frontend` folder of openshift-tests-private repo
   - ln -s ~/reops/console/frontend/packages/integration-tests-cypress upstream


**[Note] ALL following steps will run in `frontend` directory of openshift-tests-private repo**
## Install dependencies
all required dependencies are defined in `package.json` in order to run Cypress tests, run `yarn install` so that dependencies will be installed in `node_modules` folder
```bash
$ yarn install
$ ls -ltr
node_modules/     -> dependencies will be installed at runtime here
```
## Directory structure
after dependencies are installed successfully and before we run actual tests, please confirm if we have correct structure as below, two new folders will be created after above
```bash
$ ls frontend
lrwxr-xr-x  upstream -> /xxx/console/frontend/packages/integration-tests-cypress
drwxr-xr-x  node_modules
````


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
