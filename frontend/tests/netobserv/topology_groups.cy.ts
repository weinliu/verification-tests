import { netflowPage, topologySelectors, topologyPage } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"


function getTopologyResourceScopeGroupURL(groups: string): string {
    return `**/flow/metrics**groups=${groups}*`
}

describe("(OCP-53591 Network_Observability) Netflow Topology groups features", { tags: ['Network_Observability'] }, function () {

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
            catalogSources.enableQECatalogSource()
        }
        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project)
    })
    beforeEach("run before each test", function () {
        cy.clearLocalStorage()
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(3)').click()
        // check if topology view exists, if not clear filters.
        // L39-L43 removed when multiple page loads are fixed.
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
        cy.byTestID(topologySelectors.metricsDrop).should('exist').click().get('#sum').click()
        cy.contains('Display options').should('exist').click()

        // advance options menu remains visible throughout the test
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group Nodes", function () {
        const groups = 'hosts'
        cy.intercept('GET', getTopologyResourceScopeGroupURL(groups), {
            fixture: 'netobserv/flow_metrics_ghosts.json'
        })
        topologyPage.selectScopeGroup("resource", groups)
        topologyPage.isViewRendered()
        // verify number of groups, to be equal to number of cluster nodes
        cy.get(topologySelectors.nGroups).should('have.length', 6)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group Nodes+NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bnamespaces'), { fixture: 'netobserv/flow_metrics_ghostsNS.json' })
        topologyPage.selectScopeGroup("resource", "hosts+namespaces")
        topologyPage.isViewRendered()
        cy.get(topologySelectors.nGroups).should('have.length', 6)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group Nodes+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bowners'), { fixture: 'netobserv/flow_metrics_ghostsOwners.json' })
        topologyPage.selectScopeGroup("resource", "hosts+owners")
        // verify number of groups
        cy.get(topologySelectors.nGroups).should('have.length', 20)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces'), { fixture: 'netobserv/flow_metrics_gNS.json' })
        topologyPage.selectScopeGroup("resource", "namespaces")
        cy.get(topologySelectors.nGroups).should('have.length', 4)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group NS+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces%2Bowners'), { fixture: 'netobserv/flow_metrics_gNSOwners.json' })
        topologyPage.selectScopeGroup("resource", "namespaces+owners")
        cy.get(topologySelectors.nGroups).should('have.length', 20)
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
