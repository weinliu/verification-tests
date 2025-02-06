import { netflowPage, topologySelectors, topologyPage } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"

function getTopologyScopeURL(scope: string): string {
    return `**/flow/metrics**aggregateBy=${scope}*`
}

function getTopologyResourceScopeGroupURL(groups: string): string {
    return `**/flow/metrics**groups=${groups}*`
}

describe("(OCP-53591 Network_Observability) Netflow Topology groups features", { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
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

    it("(OCP-53591, memodi, Network_Observability) should verify namespace scope", function () {
        const scope = "namespace"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/flow_metrics_namespace.json'
        }).as('matchedUrl')

        // selecting something different first
        // to re-trigger API request on namespace selection
        topologyPage.selectScopeGroup("owner", null)
        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 4)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 5)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify owner scope", function () {
        const scope = "owner"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/flow_metrics_owner.json'
        }).as('matchedUrl')

        // using slider
        let lastRefresh = Cypress.$("#lastRefresh").text()
        cy.log(`last refresh is ${lastRefresh}`)
        cy.get('div.pf-c-slider__thumb').then(slider => {
            cy.wrap(slider).type('{leftarrow}', { waitForAnimations: true })
            netflowPage.waitForLokiQuery()
            cy.wait(3000)
            cy.get('#lastRefresh').invoke('text').should('not.eq', lastRefresh)
        })
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 17)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 15)
    })

    it("(OCP-53591, memodi) should verify resource scope", function () {
        const scope = 'resource'
        cy.intercept('GET', getTopologyScopeURL(scope), { fixture: 'netobserv/flow_metrics_resource.json' }).as('matchedUrl')
        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 47)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 28)
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
        cy.get(topologySelectors.nGroups).should('have.length', 10)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group Nodes+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bowners'), { fixture: 'netobserv/flow_metrics_ghostsOwners.json' })
        topologyPage.selectScopeGroup("resource", "hosts+owners")
        // verify number of groups
        cy.get(topologySelectors.nGroups).should('have.length', 11)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces'), { fixture: 'netobserv/flow_metrics_gNS.json' })
        topologyPage.selectScopeGroup("resource", "namespaces")
        cy.get(topologySelectors.nGroups).should('have.length', 4)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group NS+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces%2Bowners'), { fixture: 'netobserv/flow_metrics_gNSOwners.json' })
        topologyPage.selectScopeGroup("resource", "namespaces+owners")
        cy.get(topologySelectors.nGroups).should('have.length', 9)
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
