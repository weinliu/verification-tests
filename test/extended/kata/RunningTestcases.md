
## Running Openshift Sandboxed Containers (OSC) test cases

All the tests are under the *[sig-kata]* tag.  Default values are hard coded but can be changed (see [with a configmap](#change-the-test-defaults-with-a-configmap)).

You should have logged into the cluster.
You should run 1 test to subscribe to OSC and install kataconfig.  Test *39499* does only that:
```
bin/extended-platform-tests run all --dry-run | egrep '39499' | bin/extended-platform-tests run --timeout 120m -f -
```

This will use the defaults to subscribe to the *Openshift sandbox container Operator* from the *redhat-operators* catalog that exists in the cluster and create *kataconfig* with *kata* runtime.  If you want to change the defaults, you would delete *kataconfig* and create an *osc-config* configmap.

After this one test, you can run the full suite with (### Choosing tests to run)

The OSC test code is part of the https://github.com/openshift/openshift-tests-private repo. From the top directory, `openshift-tests-private`, kata code is in  `test/extended/kata` and the templates are in `test/extended/testdata/kata`.

Tests run using the chosen `runtimeClass` for *kataconfig* and in matching workloads. Tests that do not apply to the `runtimeClass` chosen will be skipped.  There are skips in the code for other reasons as well.
To see all the tests and if the code can skip them:
`egrep 'g.It|g.Skip|skipMessage|msg = fmt.Sprintf' test/extended/kata/kata.go`

There is a new setting, `workloadToTest` that can be *kata*, *peer-pods* or *coco*.  It is only currently used to skip or run *coco* tests.  `runtimeClass` is still used as the main way to skip tests.

### Choosing tests to run
Instead of using `egrep 'sig-kata'`, you can list all the test numbers in the regex.
examples:
- `egrep '66108|43516'`
  - check the CSV version and see if the operator is in the catalog
- `egrep 'sig-kata | egrep -iv 'upgrade|must.*gather|deletion'`
  - run the full suite, excluding upgrade, must-gather and the 2 tests that delete kataconfig.  Upgrading should be done manually

### Change the test defaults with a configmap

To override the hard coded defaults, a *configmap* named `osc-config` in the `default` *namespace* is used.

A template `testrun-cm-template.yaml` is used to create a *configmap* with `oc process`.  It is named **osc-config**.  To use the template in the example below, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`.  The template has descriptions for the optional variables.

Process:
```
oc process --ignore-unknown-parameters=true -f $L/testrun-cm-template.yaml -p OSCCHANNEL=stable NAME=osc-config NAMESPACE=default  CATSOURCENAME=<your-catalog> ICSPNEEDED=true  RUNTIMECLASSNAME=kata ENABLEPEERPODS=false OPERATORVER=1.4.1-GA  -o yaml > osc-config.yaml

oc apply -f osc-config.yaml
```

Values not specified will use the defaults from the template. The `CATSOURCENAME` should be *redhat-operators* to use the GA version or use a pre-GA custom catalog created as [below](#create-a-catalog-with-the-starting-version).
`OPERATORVER` is compared to the csv's version in test *66108*.

#### Peer-pod and CoCo testing
This testing needs other options in osc-config

Process:
```
oc process --ignore-unknown-parameters=true -f $L/testrun-cm-template.yaml -p OSCCHANNEL=stable NAME=osc-config NAMESPACE=default  CATSOURCENAME=<your-catalog> ICSPNEEDED=true  RUNTIMECLASSNAME=kata-remote ENABLEPEERPODS=true WORKLOADTOTEST=peer-pods OPERATORVER=1.8.1 -o yaml > osc-config.yaml
```
For coco, you will change `WORKLOADTOTEST=coco`.  On the 1st creation of kataconfig, trustee will be installed, configured and verified that it answers kbs-client.  If `TRUSTEE_URL=""`, the in-cluster trustee will be used to run coco tests.  If you set `TRUSTEE_URL`, that trustee will be used for attestation

You will also need to create a `peer-pods-cm`.  See the official documentation.


### Changes for libvirt related test cases on s390x
For libvirt-related test cases to run on s390x machine, a *configmap* named `peerpods-param-cm.yaml` needs to be applied before running the test. Attaching the structure of *configmap* here,

```
apiVersion: v1
kind: ConfigMap
metadata:
    name: peerpods-param-cm
    namespace: default
data:
    PROXY_TIMEOUT: "<Enter the Proxy timeout>"
    LIBVIRT_URI: "<Enter the IP address of your KVM host>"
    LIBVIRT_POOL: "<Enter the Libvirt pool name>"
    LIBVIRT_VOL_NAME: "<Enter the Libvirt volume name>"
    LIBVIRT_KVM_HOST_ADDRESS: "<Enter the IP address of your KVM host>"

```

## Upgrading
An upgrade should be done in its own testrun.  If it is combined with other tests, the tests will be in random order.  Therefore, you should create your cluster, run the upgrade test by itself and then run the full suite.

When the OLM detects the *catalog* the subscription is using has a new version, an upgrade will happen. Operators are set for automatic upgrade by default.  The subscription can have *Update Approval* set to **Manual** to prevent OLM from doing the upgrade automatically.

During this update, the new operator version will be subscribed to and the current operator will be deleted.  The subscription is otherwise unchanged

To do the upgrade, you will need a catalog where you control the image index.

### Image indexes for catalogs
The OSC build process creates an image index in quay.  You can view the tags in the repo with `skopeo list-tags docker://quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog`

Unreleased versions with have a -number from the Nth build of that version.  ex: *1.5.2-6, 1.5.2-8, 1.6.0-1, 1.6.0-8*.  Past GA versions have been tagged with the GA version.  ex: *1.4.1, 1.5.0, 1.5.1*.  Future GA versions will need to manually or automatically tag the last build without the -build number.

### Create a catalog with the starting version
To use the template example below, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`

example: _your-index_ is `quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.1`

```
oc process --ignore-unknown-parameters=true -f /catalogSourceTemplate.yaml -p NAME=_your-catalog_ IMAGEINDEX=_your index_ PUBLISHER=_your name_ DISPLAYNAME=_your name_  > catalog.yaml

oc apply -f catalog.yaml
```

In the operator hub you will see *OpenShift sandboxed containers Operator* with **your name** displayed from your **your-catalog**. When you subscribe to the operator, it will have **your-index** There will also be an operator with **Red Hat** displayed from the **redhat-operators** catalog.


##### example catalogsource created with the template
```
{
    "kind": "List",
    "apiVersion": "v1",
    "metadata": {},
    "items": [
        {
            "apiVersion": "operators.coreos.com/v1alpha1",
            "kind": "CatalogSource",
            "metadata": {
                "name": "your-catalog",
                "namespace": "openshift-marketplace"
            },
            "spec": {
                "displayName": "your name",
                "image": "quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.1",
                "publisher": "your name",
                "sourceType": "grpc"
            }
        }
    ]
}
```

### Create an `osc-config` using the catalog
[Use a configmap](#change-the-test-defaults-with-a-configmap) with _your-catalog_ as the CATSOURCENAME

### Upgrades
These are now skipped in the automation.  They should be done manually.

#### Before running further tests
After the manual upgrade, the `osc-config` configmap should be updated with `operatorVer` set to _your-next-index_'s version.  Then you can run the full sig-kata and the `66108-Version in operator CSV should match expected version` test should pass.


#### Create the `osc-config-upgrade-catalog` configmap
This is currently skipped.

The catalog that osc-config has for CATSOURCENAME will have its IMAGEINDEX changed to the one from the `osc-config-upgrade-catalog` configmap. To create the upgrade config map:

example: _your-next-index_ is `quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.2-8`

```
oc process --ignore-unknown-parameters=true -f testrunUpgradeCatalogImage.yaml -p IMAGEINDEXAFTER=_your-next-index_ > osc-config-upgrade-catalog.yaml

oc apply -f osc-config-upgrade-catalog.yaml
```

##### Run the upgrade
`bin/extended-platform-tests run all --dry-run | egrep '70824' | bin/extended-platform-tests run --timeout 120m -f - `

If the operator is not installed, this will install the starting version in `g.BeforeEach()` and then upgrade to the new version.  Otherwise, the existing operator will be upgraded.

##### Delete the `osc-config-upgrade-catalog` configmap before testing more
You _can_ run the rest of the tests with this configmap in place but it will slow the run down.  The upgrade test waits to see if the CSV changes and times out with a failure if it does not change. To avoid this, the `osc-config-upgrade-catalog` configmap should be deleted: `oc delete -n default cm osc-config-upgrade-catalog`

#### Changing the *Subscription* and channel for an upgrade
This method is currently skipped also.

This is not how Openshift does upgrades.  Early on, we used a channel in OSC for each version.  We upgraded by changing the channel in the subscription. The operator only has the `stable` channel after version *1.3*.

The "*Longduration-Author:tbuskey-High-53583-upgrade osc operator by changing subscription [Disruptive][Serial]*" testcase.

To run this test, you would install the operator with an older channel. The hard coded settings might do this or you can use the `osc-config`.  This will run all the other test cases with those values.

The channel change uses the `osc-config-upgrade-subscription`  *configmap* created with the `testrun-cm-template.yaml` [as shown](#change-the-test-defaults-with-a-configmap).  If the configmap does not exist, the test will be skipped.


[comment]: # (Spell check with aspell -c test/extended/kata/RunningTestcases.md )
