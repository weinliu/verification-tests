export const PVC = {
    "apiVersion": "v1",
    "kind": "PersistentVolumeClaim",
    "metadata": {
        "name": "loki-store",
        "namespace": "test-netobserv"
    },
    "spec": {
        "resources": {
            "requests": {
                "storage": "10G"
            }
        },
        "volumeMode": "Filesystem",
        "accessModes": [
            "ReadWriteOnce"
        ]
    }
}

export const LokiConfigMap = {
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {
        "name": "loki-config",
        "namespace": "test-netobserv"
    },
    "data": {
        "local-config.yaml": "auth_enabled: false\\nserver:\\n  http_listen_port: 3100\\n  grpc_listen_port: 9096\\ncommon:\\n  path_prefix: /loki-store\\n  storage:\\n    filesystem:\\n      chunks_directory: /loki-store/chunks\\n      rules_directory: /loki-store/rules\\n  replication_factor: 1\\n  ring:\\n    instance_addr: 127.0.0.1\\n    kvstore:\\n      store: inmemory\\nschema_config:\\n  configs:\\n    - from: 2020-10-24\\n      store: boltdb-shipper\\n      object_store: filesystem\\n      schema: v11\\n      index:\\n        prefix: index_\\n        period: 24h\\nstorage_config:\\n  filesystem:\\n    directory: /loki-store/storage\\n  boltdb_shipper:\\n    active_index_directory: /loki-store/index\\n    shared_store: filesystem\\n    cache_location: /loki-store/boltdb-cache\\n"
    }
}

export const LokiDeployment = {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "name": "loki",
        "labels": {
            "app": "loki"
        },
        "namespace": "test-netobserv"
    },
    "spec": {
        "replicas": 1,
        "selector": {
            "matchLabels": {
                "app": "loki"
            }
        },
        "template": {
            "metadata": {
                "labels": {
                    "app": "loki"
                }
            },
            "spec": {
                "securityContext": {
                    "privileged": false,
                    "capabilities": {},
                    "allowPrivilegeEscalation": false
                },
                "volumes": [
                    {
                        "name": "loki-store",
                        "persistentVolumeClaim": {
                            "claimName": "loki-store"
                        }
                    },
                    {
                        "name": "loki-config",
                        "configMap": {
                            "name": "loki-config"
                        }
                    }
                ],
                "containers": [
                    {
                        "name": "loki",
                        "image": "grafana/loki:2.4.1",
                        "volumeMounts": [
                            {
                                "mountPath": "/loki-store",
                                "name": "loki-store"
                            },
                            {
                                "mountPath": "/etc/loki",
                                "name": "loki-config"
                            }
                        ],
                        "securityContext": {
                            "allowPrivilegeEscalation": false,
                            "capabilities": {
                                "drop": [
                                    "ALL"
                                ]
                            },
                            "privileged": false,
                            "runAsNonRoot": true,
                            "seccompProfile": {
                                "type": "RuntimeDefault"
                            }
                        }
                    }
                ]
            }
        }
    }
}

export const LokiService = {
    "kind": "Service",
    "apiVersion": "v1",
    "metadata": {
        "name": "loki",
        "namespace": "test-netobserv"
    },
    "spec": {
        "selector": {
            "app": "loki"
        },
        "ports": [
            {
                "port": 3100,
                "protocol": "TCP"
            }
        ]
    }
}

export const flowcollector = {
    "apiVersion": "flows.netobserv.io/v1alpha1",
    "kind": "FlowCollector",
    "metadata": {
        "name": "cluster"
    },
    "spec": {
        "namespace": "test-netobserv",
        "agent": {
            "type": "EBPF",
            "ipfix": {
                "cacheActiveTimeout": "20s",
                "cacheMaxFlows": 400,
                "sampling": 400
            },
            "ebpf": {
                "image": "quay.io/netobserv/netobserv-ebpf-agent:main",
                "imagePullPolicy": "IfNotPresent",
                "sampling": 50,
                "cacheActiveTimeout": "5s",
                "cacheMaxFlows": 1000,
                "interfaces": [],
                "excludeInterfaces": [
                    "lo"
                ],
                "logLevel": "info",
                "privileged": false,
                "resources": {
                    "requests": {
                        "memory": "50Mi",
                        "cpu": "100m"
                    },
                    "limits": {
                        "memory": "100Mi"
                    }
                }
            }
        },
        "processor": {
            "kind": "DaemonSet",
            "port": 2055,
            "image": "quay.io/netobserv/flowlogs-pipeline:main",
            "imagePullPolicy": "IfNotPresent",
            "logLevel": "info",
            "enableKubeProbes": true,
            "healthPort": 8080,
            "prometheus": {
                "port": 9102
            },
            "ignoreMetrics": [],
            "dropUnusedFields": true,
            "resources": {
                "requests": {
                    "memory": "100Mi",
                    "cpu": "100m"
                },
                "limits": {
                    "memory": "300Mi"
                }
            }
        },
        "kafka": {
            "enable": false,
            "address": "kafka-cluster-kafka-bootstrap.test-netobserv",
            "topic": "network-flows",
            "tls": {
                "enable": false,
                "caCert": {
                    "type": "secret",
                    "name": "kafka-cluster-cluster-ca-cert",
                    "certFile": "ca.crt"
                },
                "userCert": {
                    "type": "secret",
                    "name": "flp-kafka",
                    "certFile": "user.crt",
                    "certKey": "user.key"
                }
            }
        },
        "loki": {
            "url": "http://loki.test-netobserv.svc:3100/",
            "batchWait": "1s",
            "batchSize": 102400,
            "minBackoff": "1s",
            "maxBackoff": "300s",
            "maxRetries": 10,
            "staticLabels": {
                "app": "netobserv-flowcollector"
            },
            "tls": {
                "enable": false,
                "caCert": {
                    "type": "configmap",
                    "name": "loki",
                    "certFile": "service-ca.crt"
                }
            }
        },
        "consolePlugin": {
            "register": true,
            "image": "quay.io/netobserv/network-observability-console-plugin:main",
            "imagePullPolicy": "IfNotPresent",
            "port": 9001,
            "logLevel": "info",
            "portNaming": {
                "enable": true,
                "portNames": {
                    "3100": "loki"
                }
            }
        },
        "clusterNetworkOperator": {
            "namespace": "openshift-network-operator"
        },
        "ovnKubernetes": {
            "namespace": "ovn-kubernetes",
            "daemonSetName": "ovnkube-node",
            "containerName": "ovnkube-node"
        }
    }
}