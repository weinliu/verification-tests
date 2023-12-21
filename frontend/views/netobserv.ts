import { operatorHubPage } from "../views/operator-hub-page"

export const project = "netobserv"

export const Operator = {
    name: () => {
        if (Cypress.env('noo_catalog_src') == "upstream") {
            return "NetObserv Operator"
        }
        else {
            return "Network Observability"
        }
    },
    install: (catalogSourceDisplayName: string) => {
        cy.visit(`/k8s/ns/openshift-netobserv-operator/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
        //  don't install again if its already installed
        cy.get("div.loading-box").should('be.visible').then(loading => {
            if (Cypress.$('td[role="gridcell"]').length == 0) {
                operatorHubPage.goTo()
                const catalogSourceSelectorCheckbox = `input[title="${catalogSourceDisplayName}"]`
                cy.get(catalogSourceSelectorCheckbox).check()
                operatorHubPage.install(Operator.name(), true)
            }
        })
    },
    enableAllFLPMetrics: () => {
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_includeList_accordion-toggle').should('exist').click()
        cy.enableFLPMetrics([
            "node_flows_total",
            "node_ingress_bytes_total",
            "node_egress_bytes_total",
            "node_ingress_packets_total",
            "node_egress_packets_total",
            // enable all namespace metrics
            "namespace_flows_total",
            "namespace_ingress_bytes_total",
            "namespace_egress_bytes_total",
            "namespace_ingress_packets_total",
            "namespace_egress_packets_total",
            // enable all workload metrics
            "workload_flows_total",
            "workload_ingress_bytes_total",
            "workload_egress_bytes_total",
            "workload_ingress_packets_total",
            "workload_egress_packets_total",
          ]);
    },
    visitFlowcollector: () => {
        // this assumes Loki is already deployed in netobserv NS
        cy.visit('k8s/ns/openshift-netobserv-operator/operators.coreos.com~v1alpha1~ClusterServiceVersion')
        const selector = '[data-test-operator-row="' + Operator.name() + '"]'
        cy.get(selector).invoke('attr', 'href').then(href => {
            cy.visit(href)
        })

        cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
    },
    createFlowcollector: (namespace, parameters?: string) => {
        Operator.visitFlowcollector()
        cy.get('div.loading-box__loaded:nth-child(2)').should('exist')
        cy.wait(5000)
        cy.get("#yaml-create").should('exist').then(() => {
            if ((Cypress.$('td[role="gridcell"]').length > 0) && (parameters != null)) {
                Operator.deleteFlowCollector()
                // come back to flowcollector tab after deletion
                Operator.visitFlowcollector()
            }
        })
        // don't create flowcollector if already exists
        cy.get('div.loading-box:nth-child(1)').should('be.visible').then(() => {
            if (Cypress.$('td[role="gridcell"]').length == 0) {
                cy.adminCLI(`oc new-project ${namespace}`)
                // deploy loki
                cy.adminCLI(`oc create -f ./fixtures/netobserv/loki.yaml -n ${namespace}`)
                cy.byTestID('item-create').should('exist').click()
                cy.get('#form').click() // bug in console where yaml view is default
                Operator.configureEbpfAgent()
                if (parameters == "PacketDrop") {
                    Operator.enablePacketDrop()
                }
                if (parameters == "FlowRTT") {
                    Operator.enableFlowRTT()
                }
                if (parameters == "DNSTracking") {
                    Operator.enableDNSTracking()
                }
                Operator.configureLoki(namespace)
                cy.get('#root_spec_namespace').clear().type(namespace)
                if (parameters == "Conversations") {
                    Operator.enableConversations()
                }
                if (parameters == "AllMetrics") {
                    Operator.enableAllFLPMetrics()
                }
                cy.byTestID('create-dynamic-form').click()
                cy.wait(5000)
                cy.byTestID('status-text').should('exist').should('contain.text', 'Ready')
                cy.byTestID('status-text').should('exist').should('contain.text', 'FLPMonolithReady')
                cy.byTestID('status-text').should('exist').should('contain.text', 'FLPParentReady')
                cy.byTestID('status-text').should('exist').should('contain.text', 'FlowCollectorLegacyReady')
                cy.byTestID('status-text').should('exist').should('contain.text', 'MonitoringReady')

                cy.byTestID('refresh-web-console', { timeout: 60000 }).should('exist')
                // for OCP < 4.12 refresh-web-console element doesn't exist, use toast-action instead.
                // cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
                cy.reload(true)
            }
        })
    },
    configureEbpfAgent: () => {
        cy.get('#root_spec_agent_accordion-toggle').click()
        cy.get('#root_spec_agent_type').click().then(agent => {
            cy.contains("eBPF").should('exist')
            cy.contains("IPFIX").should('exist')
            cy.get('#eBPF-link').click()
        })
        cy.get('#root_spec_agent_ebpf_accordion-toggle').click()
        cy.get('#root_spec_agent_ebpf_sampling').clear().type('1')
    },
    enablePacketDrop: () => {
        cy.get('#root_spec_agent_ebpf_privileged').click()
        cy.get('#root_spec_agent_ebpf_features_accordion-toggle').click()
        cy.get('#root_spec_agent_ebpf_features_add-btn').click()
        cy.get('#root_spec_agent_ebpf_features_0').click().then(features => {
            cy.contains("PacketDrop").should('exist')
            cy.get('#PacketDrop-link').click()
        })
        // Deploy PacketDrop metrics to includeList
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_includeList_accordion-toggle').should('exist').click()
        cy.enableFLPMetrics([
            "namespace_drop_bytes_total",
            "namespace_drop_packets_total",
            "node_drop_bytes_total",
            "node_drop_packets_total",
            "workload_drop_bytes_total",
            "workload_drop_packets_total"
        ]);
    },
    enableDNSTracking: () => {
        cy.get('#root_spec_agent_ebpf_features_accordion-toggle').click()
        cy.get('#root_spec_agent_ebpf_features_add-btn').click()
        cy.get('#root_spec_agent_ebpf_features_0').click().then(features => {
            cy.contains("DNSTracking").should('exist')
            cy.get('#DNSTracking-link').click()
        })
        // Deploy DNS metrics to includeList
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_includeList_accordion-toggle').should('exist').click()
        cy.enableFLPMetrics([
            "namespace_dns_latency_seconds",
            "node_dns_latency_seconds",
            "workload_dns_latency_seconds"
        ]);
    },
    enableFlowRTT: () => {
        cy.get('#root_spec_agent_ebpf_features_accordion-toggle').click()
        cy.get('#root_spec_agent_ebpf_features_add-btn').click()
        cy.get('#root_spec_agent_ebpf_features_0').click().then(features => {
            cy.contains("FlowRTT").should('exist')
            cy.get('#FlowRTT-link').click()
        })
        // Deploy FlowRTT metrics to includeList
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_includeList_accordion-toggle').should('exist').click()
        cy.enableFLPMetrics([
            "namespace_rtt_seconds",
            "node_rtt_seconds",
            "workload_rtt_seconds"
        ]);
    },
    configureLoki: (namespace: string) => {
        cy.get('#root_spec_loki_accordion-toggle').click()
        cy.get('#root_spec_loki_mode').click().then(moreOpts => {
            cy.contains("Manual").should('exist')
            cy.contains("Microservices").should('exist')
            cy.contains("Monolithic").should('exist')
            cy.contains("LokiStack").should('exist')
            cy.get('#Monolithic-link').click()
        })
        cy.get('#root_spec_loki_monolithic_accordion-toggle').click()
        cy.get('#root_spec_loki_monolithic_url').clear().type(`http://loki.${namespace}.svc:3100/`)
    },
    enableConversations: () => {
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_logTypes').click().then(moreOpts => {
            cy.contains("Flows").should('exist')
            cy.contains("Conversations").should('exist')
            cy.contains("EndedConversations").should('exist')
            cy.contains("All").should('exist')
            cy.get('#All-link').click()
        })
    },
    deleteFlowCollector: () => {
        cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')
        // cy.byLegacyTestID('resource-title').should('exist')
        cy.contains('Flow Collector').should('exist').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.byTestID('cluster').should('exist').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.byLegacyTestID('actions-menu-button').should('exist').click()
        cy.byTestActionID('Delete FlowCollector').should('exist').click()
        cy.byTestID('confirm-action').should('exist').click()
        cy.adminCLI(`oc delete -f ./fixtures/netobserv/loki.yaml -n ${project}`)
        cy.adminCLI(`oc delete project ${project}`)
        cy.byTestID('refresh-web-console', { timeout: 60000 }).should('exist')
        // for OCP < 4.12 refresh-web-console element doesn't exist, use toast-action instead.
        // cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
        cy.reload(true)
    },
    uninstall: () => {
        cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')

        cy.contains(Operator.name()).should('exist').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.get('.co-actions-menu > .pf-c-dropdown__toggle').should('exist').click()
        cy.byTestActionID('Uninstall Operator').should('exist').click()
        cy.byTestID('confirm-action').should('exist').click()
    },
    deleteCatalogSource: (catalogSource: string) => {
        cy.visit('k8s/cluster/config.openshift.io~v1~OperatorHub/cluster/sources')
        cy.byTestID(catalogSource).should('exist').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.get('.co-actions-menu > .pf-c-dropdown__toggle').should('exist').click()
        cy.byTestActionID('Delete CatalogSource').should('exist').click()
        cy.byTestID('confirm-action').should('exist').click()
    }
}

Cypress.Commands.add('enableFLPMetrics', (tags: string[]) => {
    for (let i = 0; i < tags.length; i++) {
        const tag = tags[i];
        cy.get('#root_spec_processor_metrics_includeList_add-btn').should('exist').click()
        cy.get(`#root_spec_processor_metrics_includeList_${i}`).should('exist').click().then(metrics => {
            cy.get(`#${tag}-link`).should('exist').click()
        })
    }
});

declare global {
    namespace Cypress {
        interface Chainable {
            enableFLPMetrics(tag: string[]): Chainable<Element>
        }
    }
}
