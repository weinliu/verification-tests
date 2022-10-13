import { PVC, LokiConfigMap, LokiDeployment, LokiService, flowcollector } from "../fixtures/flowcollector"
import { OCCli, OCCreds } from "./cluster-cliops"
import { operatorHubPage } from "../views/operator-hub-page"
import { netflowPage, genSelectors } from "../views/netflow-page"


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
    createCustomCatalog: (image: any, catalogSource = "netobserv-test") => {
        cy.visit('k8s/cluster/config.openshift.io~v1~OperatorHub/cluster/sources')
        if (catalogSource != "community-operators" && image == null) {
            throw new Error("Operator catalog image must be specified for catalogSource other than community-operator");
        }

        if (catalogSource != "community-operators") {
            cy.byTestID('item-create').should('exist').click()
            cy.byTestID('catalog-source-name').type(catalogSource)
            cy.get('#catalog-source-display-name').type('NetObserv QE')
            cy.get('#catalog-source-publisher').type('ocp-qe')
            cy.byTestID('catalog-source-image').type(image)
            cy.byTestID('save-changes').click()
        }

        cy.byTestID(catalogSource).should('exist')
        cy.byTestID(catalogSource + '-status', { timeout: 60000 }).should('have.text', "READY")
    },
    install: (catalogSourceDisplayName: string) => {
        operatorHubPage.goTo()
        operatorHubPage.isLoaded()
        const catalogSourceSelectorCheckbox = `input[title="${catalogSourceDisplayName}"]`
        cy.get(catalogSourceSelectorCheckbox).check()
        operatorHubPage.install("NetObserv Operator")
    },
    createFlowcollector: (namespace: string) => {
        // this assumes Loki is already deployed in netobserv NS
        cy.visit('k8s/ns/openshift-operators/operators.coreos.com~v1alpha1~ClusterServiceVersion')
        cy.get('[data-test-operator-row="NetObserv Operator"]').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })

        cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })

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

        cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
        cy.reload(true)

        netflowPage.visit()
        cy.byTestID(genSelectors.refreshDrop, { timeout: 30000 }).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="15s"]').should('exist').click()
            })
        })

        cy.byTestID("table-composable", { timeout: 120000 }).should('exist')
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

        cy.contains('NetObserv Operator').should('exist').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.get('.co-actions-menu > .pf-c-dropdown__toggle').should('exist').click()
        cy.byTestActionID('Uninstall Operator').should('exist').click()
        cy.byTestID('confirm-action').should('exist').click()
    },
    deleteCatalogSource: (catalogSource: string) => {
        cy.visit('k8s/cluster/config.openshift.io~v1~OperatorHub/cluster/sources')
        cy.byTestID(catalogSource).invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.get('.co-actions-menu > .pf-c-dropdown__toggle').should('exist').click()
        cy.byTestActionID('Delete CatalogSource').should('exist').click()
        cy.byTestID('confirm-action').should('exist').click()
    }
}
