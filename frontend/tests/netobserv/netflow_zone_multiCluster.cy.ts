import { colSelectors, netflowPage, topologyPage, topologySelectors } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"

function getTopologyScopeURL(scope: string): string {
    return `**/flow/metrics**aggregateBy=${scope}*`
}

describe('Netflow Zone and multiCluster test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "ZonesAndMultiCluster")
    })

    beforeEach('any netflow zone and multiCluster test', function () {
        netflowPage.visit()
    })

    it("(OCP-71525, aramesha, Network_Observability) should validate zone/multiCluster columns", function () {
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')

        cy.openColumnsModal().then(col => {
            cy.get(colSelectors.columnsModal).should('be.visible')
            // Check zone columns
            cy.get('#SrcZone').check()
            cy.get('#DstZone').check()

            // Check multiCluster column
            cy.get('#ClusterName').check()
            cy.byTestID(colSelectors.save).click()
        })

        cy.byTestID('table-composable').should('exist').within(() => {
            // Verify zone column
            cy.get(colSelectors.SrcZone).should('exist')
            cy.get(colSelectors.DstZone).should('exist')

            // Verify multiCluster column
            cy.get(colSelectors.ClusterName).should('exist')
        })
    })

    it("(OCP-71524, aramesha, Network_Observability) should verify zone/cluster scope topology", function () {
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
            cy.contains('Display options').should('exist').click()
        })

        // Verify Zone scope
        var scope = "zone"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/flow_metrics_zone.json'
        }).as('matchedUrl')

        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()

        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 6)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 4)

        // Verify Cluster scope
        scope = "cluster"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/flow_metrics_cluster.json'
        }).as('matchedUrl')

        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()

        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 0)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 1)
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("delete flowcollector and NetObs Operator", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
