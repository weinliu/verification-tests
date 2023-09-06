import { Operator } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { dashboard, graphSelector } from "views/dashboards-page"

// if project name is changed here, it also needs to be changed 
// under fixture/flowcollector.ts and topology_view.spec.ts
const project = 'netobserv'


describe('NETOBSERV dashboards tests', { tags: ['NETOBSERV'] }, function () {

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

        var panels: string[] = ['rates-chart', 'percentage-of-flows-generated-by-netobserv-own-traffic-chart', '-chart']
        cy.checkDashboards(panels)

        var appsInfra: string[] = ["applications-chart", "infrastructure-chart"]
        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-namespaces-1-min-rates').should('exist').within(topflow => {
            cy.checkDashboards(appsInfra)
        })

        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-workloads-1-min-rates').should('exist').within(topflow => {
            cy.checkDashboards(appsInfra)
        })

        var cpuMemory: string[] = ['cpu-usage-chart', 'memory-usage-chart']

        cy.byLegacyTestID('panel-agents').should('exist').within(agent => {
            cy.checkDashboards(cpuMemory)
        })

        cy.byLegacyTestID('panel-processor').should('exist').within(processor => {
            cy.checkDashboards(cpuMemory)
        })

        cy.get('[data-test="operator-reconciliation-rate-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    })

    it("OCP-63790, should have flow based dashboards", function () {
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")
        cy.byTestID("-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        var appsInfra: string[] = ["applications-chart", "infrastructure-chart"]
        cy.byLegacyTestID('panel-top-byte-rates-received-per-source-and-destination-namespaces').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })
        cy.byLegacyTestID('panel-top-byte-rates-received-per-source-and-destination-workloads').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })
    })

    after("delete flowcollector and NetObs Operator", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()
    })

})