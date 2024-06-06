import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"
import { dashboard, graphSelector } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "RTT"
]

const flowRTTPanels = [
    // below 2 panels should appear with the 'node_rtt_seconds' metric
    "top-p50-srtt-per-node-(ms)-chart",
    "top-p99-srtt-per-node-(ms)-chart",
    // below 2 panels should appear with the 'namespace_rtt_seconds' metric
    "top-p50-srtt-per-infra-namespace-(ms)-chart",
    "top-p99-srtt-per-infra-namespace-(ms)-chart",
    // below 2 panels should appear with the 'workload_rtt_seconds' metric
    "top-p50-srtt-per-infra-workload-(ms)-chart",
    "top-p99-srtt-per-infra-workload-(ms)-chart"
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
        cy.byTestID("scope-dropdown").click().byTestID("host").click()
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
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        // verify 'TCP latency,p99' panel
        cy.get('[data-test="tcp-latency,-p99-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        cy.checkDashboards(flowRTTPanels)
    })
    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})

