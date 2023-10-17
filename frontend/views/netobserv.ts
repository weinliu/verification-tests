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
    enableFLPMetrics: () => {
        // this will need to change once NETOBSERV-1284 is fixed.
        cy.get('#root_spec_processor_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_accordion-toggle').click()
        cy.get('#root_spec_processor_metrics_ignoreTags_accordion-toggle').should('exist').click()
        cy.enableFLPMetric("namespaces-flows")
        cy.enableFLPMetric("workloads-flows")
        cy.enableFLPMetric("nodes-flows")
        cy.enableFLPMetric("namespaces")
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
                //come back to flowcollector tab after deletion
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
                Operator.configureLoki(namespace)
                Operator.enableFLPMetrics()
                cy.get('#root_spec_namespace').clear().type(namespace)
                if (parameters == "Conversations") {
                    Operator.enableConversations()
                }
                cy.byTestID('create-dynamic-form').click()
                cy.byTestID('status-text').should('exist').should('have.text', 'Ready')

                cy.byTestID('refresh-web-console', { timeout: 60000 }).should('exist')
                // for OCP < 4.12 refresh-web-console element doesn't exist, use toast-action instead.
                // cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
                cy.reload(true)
            }
        })
    },
    configureEbpfAgent: () => {
        cy.get('#root_spec_agent_accordion-toggle').click()
        cy.get('#root_spec_agent_type').should('have.text', 'EBPF')
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
    },
    configureLoki: (namespace: string) => {
        cy.get('#root_spec_loki_accordion-toggle').click()
        cy.get('#root_spec_loki_url').clear().type(`http://loki.${namespace}.svc:3100/`)
    },
    enableConversations: () => {
        cy.get('#root_spec_processor_logTypes').click().then(moreOpts => {
            cy.contains("FLOWS").should('exist')
            cy.contains("ENDED_CONVERSATIONS").should('exist')
            cy.contains("CONVERSATIONS").should('exist')
            cy.contains("ALL").should('exist')
            cy.get('#ALL-link').click()
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
Cypress.Commands.add('enableFLPMetric', (tag: string) => {
    cy.get(`[value=\"${tag}\"]`).parent().parent().within(tag => {
        cy.get('.co-dynamic-form__array-field-group-remove > button').should('exist').click()
    })
});

declare global {
    namespace Cypress {
        interface Chainable {
            enableFLPMetric(tag: string): Chainable<Element>
        }
    }
}
