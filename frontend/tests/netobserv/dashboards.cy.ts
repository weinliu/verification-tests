import { Operator } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { dashboard, graphSelector } from "views/dashboards-page"

// if project name is changed here, it also needs to be changed 
// under fixture/flowcollector.ts and topology_view.spec.ts
const project = 'netobserv'


describe('NetObserv dashboards tests', { tags: ['NETOBSERV'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        // sepcify --env noo_release=upstream to run tests 
        // from most recent "main" image
        let catalogImg
        let catalogDisplayName = "Production Operators"
        const catSrc = Cypress.env('noo_catalog_src')
        if (catSrc == "upstream") {
            catalogImg = 'quay.io/netobserv/network-observability-operator-catalog:v0.0.0-main'
            this.catalogSource = "netobserv-test"
            catalogDisplayName = "NetObserv QE"
            catalogSources.createCustomCatalog(catalogImg, this.catalogSource, catalogDisplayName)
        }
        else {
            catalogSources.enableQECatalogSource(this.catalogSource, catalogDisplayName)
        }
        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project)
    })

    it('OCP-61893, should have health dashboards', function () {
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-health")
        cy.byTestID('rates-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        cy.byLegacyTestID('panel-agents').should('exist').within(agent => {

            cy.byTestID('cpu-usage-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
            cy.byTestID('memory-usage-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        })

        cy.byLegacyTestID('panel-processor').should('exist').within(processor => {
            cy.byTestID('cpu-usage-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
            cy.byTestID('memory-usage-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        })

        cy.get('[data-test="operator-reconciliation-rate-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    })

    it("OCP-63790, should have flow based dashboards", function () {
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv")
        cy.byTestID("top-flow-rates-per-source-and-destination-namespaces-(1-min-rates)-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        cy.byTestID("top-byte-rates-received-per-source-and-destination-nodes-(1-min-rates)-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        cy.byTestID("top-byte-rates-received-per-source-and-destination-workloads-(1-min-rates)-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    })

    after("delete flowcollector and NetObs Operator", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()
    })

})