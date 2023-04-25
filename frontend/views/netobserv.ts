import { PVC, LokiConfigMap, LokiDeployment, LokiService, flowcollector } from "../fixtures/flowcollector"
import { OCCli, OCCreds } from "./cluster-cliops"
import { operatorHubPage } from "../views/operator-hub-page"

export class NetObserv {
    creds: OCCreds
    oc: OCCli

    constructor(oc?: OCCli, creds?: OCCreds) {
        this.creds = creds || undefined
        if (creds) {
            this.oc = new OCCli(creds)
        }
        else if (!oc) {
            throw 'must pass creds: OCCreds property or oc: OCli'
        }
        else {
            this.oc = oc

        }
    }

    deploy_loki(): void {
        this.oc.apply_manifest(PVC)
        this.oc.apply_manifest(LokiConfigMap)
        this.oc.apply_manifest(LokiDeployment)
        this.oc.apply_manifest(LokiService)
    }
    undeploy_loki(): void {
        this.oc.delete_resources(LokiService)
        this.oc.delete_resources(LokiDeployment)
        this.oc.delete_resources(LokiConfigMap)
        this.oc.delete_resources(PVC)
    }

    deploy_flowcollector(): void {
        this.oc.apply_manifest(flowcollector)
    }
}
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
    createFlowcollector: (namespace: string) => {
        // this assumes Loki is already deployed in netobserv NS
        cy.visit('k8s/ns/openshift-netobserv-operator/operators.coreos.com~v1alpha1~ClusterServiceVersion')
        const selector = '[data-test-operator-row="' + Operator.name() + '"]'
        cy.get(selector).invoke('attr', 'href').then(href => {
            cy.visit(href)
        })

        cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        // don't create flowcollector if already exists
        cy.get('div.loading-box:nth-child(1)').should('be.visible').then(() => {
            if (Cypress.$('td[role="gridcell"]').length == 0) {
                cy.adminCLI(`oc new-project ${namespace}`)
                // deploy loki
                cy.adminCLI(`oc create -f ./fixtures/netobserv/loki.yaml -n ${namespace}`)
                cy.byTestID('item-create').should('exist').click()
                cy.get('#form').click() // bug in console where yaml view is default
                cy.get('#root_spec_agent_accordion-toggle').click()
                cy.get('#root_spec_agent_type').should('have.text', 'EBPF')
                cy.get('#root_spec_agent_ebpf_accordion-toggle').click()
                cy.get('#root_spec_agent_ebpf_sampling').clear().type('1')
                cy.get('#root_spec_loki_accordion-toggle').click()
                cy.get('#root_spec_loki_url').clear().type(`http://loki.${namespace}.svc:3100/`)
                cy.get('#root_spec_namespace').clear().type(namespace)
                cy.byTestID('create-dynamic-form').click()
                cy.byTestID('status-text').should('exist').should('have.text', 'Ready')

                cy.byTestID('refresh-web-console', { timeout: 60000 }).should('exist')
                // for OCP <= 4.12 refresh-web-console element doesn't exist, use toast-action instead.
                // cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
                cy.reload(true)
            }
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
