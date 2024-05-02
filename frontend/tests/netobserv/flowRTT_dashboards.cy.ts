import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"
import { dashboard, graphSelector, appsInfra } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "RTT"
]

describe('(OCP-68246 Network_Observability) FlowRTT test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

        // specify --env noo_release=upstream to run tests 
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
        Operator.createFlowcollector(project, "FlowRTT")
    })

    it("(OCP-68246, aramesha, Network_Observability) Validate flowRTT edge labels and Query Summary stats", function () {
        netflowPage.visit()
        cy.clearLocalStorage()
        netflowPage.selectSourceNS(project)
        cy.get('#tabs-container li:nth-child(3)').click()
        // check if topology view exists, if not clear filters.
        // this can be removed when multiple page loads are fixed.
        if (Cypress.$('[data-surface=true][transform="translate(0, 0) scale(1)]').length > 0) {
            cy.log("need to clear all filters")
            cy.get('[data-test="filters"] > [data-test="clear-all-filters-button"]').should('exist').click()
        }
        cy.get('#drawer').should('not.be.empty')

        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('layout-dropdown').click()
            cy.byTestID('Grid').click()
        })

        cy.byTestID('metricType').should('exist').click()
        cy.get('#metricType > ul > li').should('have.length', 3).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })

        cy.get('li#TimeFlowRttNs').click()
        cy.contains('Display options').should('exist').click()

        // validate edge labels shows flowRTT info
        cy.get('#zoom-in').click({ force: true }).click({ force: true }).click({ force: true });

        cy.get('[data-test-id=edge-handler]').each((g) => {
            expect(g.text()).to.match(/\d* ms/gm);
        });

        // verify Query summary panel
        cy.get(querySumSelectors.avgRTT).should('exist').then(avgRTT => {
            cy.checkQuerySummary(avgRTT)
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-68246, aramesha, Network_Observability) Validate flowRTT dashboards", function () {
        // navigate to 'NetObserv' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

        // verify 'Round-trip time per node (milliseconds - p99 and p50)' panel
        // panel should appear with the flowcollector metric 'node_rtt_seconds'
        cy.byLegacyTestID('panel-round-trip-time-per-node-milliseconds-p-99-and-p-50').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Round-trip time per namespace (milliseconds - p99 and p50)' panel
        // panel should appear with the flowcollector metric 'namespace_rtt_seconds'
        cy.byLegacyTestID('panel-round-trip-time-per-namespace-milliseconds-p-99-and-p-50').should('exist').within(FlowRTT => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Round-trip time per workload (milliseconds - p99 and p50)' panel
        // panel should appear with the flowcollector metric 'workload_rtt_seconds'
        cy.byLegacyTestID('panel-round-trip-time-per-workload-milliseconds-p-99-and-p-50').should('exist').within(FlowRTT => {
            cy.checkDashboards(appsInfra)
        })
    })
    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})

