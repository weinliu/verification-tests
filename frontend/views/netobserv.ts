import { OperatorGroup, Subscription } from "../fixtures/noo_subscription"
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

    deploy_operator(namespace: string): void {
        this.oc.create_project(namespace)
        this.oc.apply_manifest(OperatorGroup)
        this.oc.apply_manifest(Subscription)
    }

    deploy_loki(): void {
        this.oc.apply_manifest(PVC)
        this.oc.apply_manifest(LokiConfigMap)
        this.oc.apply_manifest(LokiDeployment)
        this.oc.apply_manifest(LokiService)
    }

    deploy_flowcollector(): void {
        this.oc.apply_manifest(flowcollector)
    }
}

export const Operator = {
    install: () => {
        operatorHubPage.goTo()
        operatorHubPage.isLoaded()
        operatorHubPage.install("NetObserv Operator")
        cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')
    },
    createFlowcollector: () => {
        cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })

        cy.byTestID('item-create').should('exist').click()
        cy.get('#root_spec_ipfix_accordion-toggle').click()
        cy.get('#root_spec_ipfix_sampling').clear().type('2')
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
        cy.byLegacyTestID('resource-title').should('exist')
        cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.byLegacyTestID('kebab-button').first().click()
        cy.byTestActionID('Delete FlowCollector').click()
        cy.byTestID('confirm-action').click()
    },
    uninstall: () => {
        cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')
        cy.byLegacyTestID('resource-title').should('exist')

        cy.contains('NetObserv Operator').invoke('attr', 'href').then(href => {
            cy.visit(href)
        })
        cy.get('.co-actions-menu > .pf-c-dropdown__toggle').click()
        cy.byTestActionID('Uninstall Operator').click()
        cy.byTestID('confirm-action').click()
    }
}
