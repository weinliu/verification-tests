# OpenShift Console tests

## Directory Structure
```bash
$ ls frontend
README.md
tests/    -> Cypress auto tests scenario
plugins/  -> webpack-preprocessor, enviornment variables, baseUrl, custom tasks
upstream@ -> dynamically created soft link to upstream helpers 
```

## Local Development
### Clone upstream 'console' repo somewhere and install dependencies
```bash
git clone git@github.com:openshift/console.git
cd console/frontend; yarn install
```
### Create Soft link
```bash
cd /path/to/openshift-tests-private/frontend
ln -s /path/to/upstream/console/frontend/packages/integration-tests-cypress upstream
```
### Install all dependencies
```bash
yarn install
ls -ltr
node_modules/     -> dependencies will be installed at runtime here
```
### Export necessary variables
```bash
export BRIDGE_BASE_ADDRESS=https://<console_route_spec_host>
export LOGIN_IDP=kube:admin
export LOGIN_USERNAME=testuser
export LOGIN_PASSWORD=testpassword
export KUBECONFIG_PATH=/path/to/kubeconfig
export LOGIN_UP_PAIR=uiauto1:redhat(optional)
```
### Start Cypress and add/run/debug your tests
```bash
npx cypress open
npx cypress run --env grep="Smoke"

```
