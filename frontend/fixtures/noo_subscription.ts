
export const OperatorGroup = {
  "apiVersion": "operators.coreos.com/v1",
  "kind": "OperatorGroup",
  "metadata": {
    "name": "netobserv",
    "namespace": "network-observability"
  },
  "spec": {
    "targetNamespaces": [
      "network-observability"
    ]
  }
}


export const Subscription = {
  "apiVersion": "operators.coreos.com/v1alpha1",
  "kind": "Subscription",
  "metadata": {
    "name": "netobserv-operator",
    "namespace": "network-observability"
  },
  "spec": {
    "channel": "alpha",
    "name": "netobserv-operator",
    "source": "community-operators",
    "sourceNamespace": "openshift-marketplace"
  }
}

export const MainCatalogSource = {
  "apiVersion": "operators.coreos.com/v1alpha1",
  "kind": "CatalogSource",
  "metadata": {
    "name": "netobserv-testing",
    "namespace": "openshift-marketplace"
  },
  "spec": {
    "displayName": "NetObservTest",
    "image": "quay.io/netobserv/network-observability-operator-catalog:vmain",
    "sourceType": "grpc"
  }
}

export const MainSubscription = {
  "apiVersion": "operators.coreos.com/v1alpha1",
  "kind": "Subscription",
  "metadata": {
    "name": "netobserv-operator",
    "namespace": "network-observability"
  },
  "spec": {
    "channel": "alpha",
    "name": "netobserv-operator",
    "source": "netobserv-testing",
    "sourceNamespace": "openshift-marketplace"
  }
}