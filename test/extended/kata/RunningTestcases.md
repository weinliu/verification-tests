
# Running Openshift Sandboxed Containers (OSC) testcases

All the tests are under the [sig-kata] tag.  Default values are hardcoded but can be changed.

```
bin/extended-platform-tests run all --dry-run | egrep 'sig-kata' | bin/extended-platform-tests run --timeout 120m -f -
```

This will subscribe to Openshift Sandbox Containers from the *redhat-operators* catalog that exists in the cluster and create *kataconfig* with *kata* runtime.  It will run all the tests that are for kata runtime.

The OSC test code is part of the https://github.com/openshift/openshift-tests-private repo.  From the top directory, `openshift-tests-private`, kata code is in  `test/extended/kata` and the templates are in `test/extended/testdata/kata`.

Tests run using one of the available `runtimeClass` for *kataconfig* and workloads. Tests that do not apply to a `runtimeClass` will be skipped.

## Change the test defaults with a configmap

A *configmap* named `osc-config` in the `default` *namespace* is used.  The hardcoded variable setting will be used unless they are set in the configmap.

The template `testrun-cm-template.yaml` is used to create a *configmap*.  It should be named **osc-config**.  To use the template, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`


### Process the template:

`oc process --ignore-unknown-parameters=true -f $L/testrun-cm-template.yaml -p OSCCHANNEL=stable NAME=osc-config NAMESPACE=default  CATSOURCENAME=redhat-operators ICSPNEEDED=true  RUNTIMECLASSNAME=kata ENABLEPEERPODS=false OPERATORVER=1.4.1-GA  -o yaml > osc-config.yaml`

Apply `oc apply -f osc-config.yaml`

Note that the above will use the GA version of OSC from the *redhat-operators* on the cluster. If you want a pre-GA version, see *Create a catalogsource* below and change `CATSOURCENAME` to use your catalog

`OPERATORVER` is compared to the csv's version in test 66108.

## Change the test defaults with environment variables


Environment variables are checked after the `osc-config` *configmap*.  Any that are set will override hardcoded and configmap variables.

The environment variables start with **OSCS**.  Example:
```
export OSCSOSCCHANNEL=stable-1.3
export OSCSICSPNEEDED=false
export OSCSCATSOURCENAME=redhat-operators
export OSCSMUSTGATHERIMAGE="registry.redhat.io/openshift-sandboxed-containers/osc-must-gather-rhel8:1.3.0"
export OSCSOPERATORVER="1.3.0"
```

Note that the above will use the GA version of OSC from the *redhat-operators* on the cluster. If you want a pre-GA version, see *create a catalogsource* below and change `OSCSCATSOURCENAME` to use your catalog

## Changing the operator channel for an *upgrade*

Early on, changing the channel was considered to be an upgrade.  Operators have an automatic upgrade unless the subscription has *Update Approval* set to **Manual**.

Changing the channel is a different process.  The more recent operator version use only the `stable` channel.

The "*Longduration-Author:tbuskey-High-53583-upgrade osc operator [Disruptive][Serial]*" testcase.

To run this test, you would install the operator with an older channel.  The hardcoded settings might do this or you can use the `osc-config` or environment variables.  This will run all the other test cases with those values.

The channel change uses either the `osc-config-upgrade`  *configmap* created with the same template.  The environment variables starting with  **OSCU** instead of **OSCS** can also be used.  If neither is used, the test will be skipped.

## Create a catalogsource
Usually you will create a *catalogsource* from a template that has your build instead of *redhat-operators*.

To use the template, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`

### Process the *catalogSourceTemplate.yaml* template:

`oc process --ignore-unknown-parameters=true -f $L/catalogSourceTemplate.yaml -p NAME=mycatalog IMAGEINDEX=quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.0-28 > catalog.json`

Apply: `oc apply -f catalog.json`

In the operator hub you will see *OpenShift sandboxed containers Operator* with **QE** displayed from your **mycatalog**.  There will also be one with **Red Hat** displayed from the redhat-operators catalog.


#### example catalogsource created with the template
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
                "name": "mycatalog",
                "namespace": "openshift-marketplace"
            },
            "spec": {
                "displayName": "QE",
                "image": "quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.0-28",
                "publisher": "QE",
                "sourceType": "grpc"
            }
        }
    ]
}
```



