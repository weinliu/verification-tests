import { netflowPage, topologySelectors, topologyPage } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"

function getTopologyResourceScopeGroupURL(groups: string): string {
    return `**/flow/metrics**groups=${groups}*`
}

describe("(OCP-53591 Network_Observability) Netflow Topology edges,labels, badges features", { tags: ['Network_Observability'] }, function () {

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
        cy.byTestID(topologySelectors.metricsDrop).should('exist').click().get('#sum').click()
        cy.contains('Display options').should('exist').click()

        // advance options menu remains visible throughout the test
        topologyPage.selectScopeGroup("resource", "owners")
        cy.contains('Display options').should('exist').click()
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group owners", function () {
        cy.intercept(getTopologyResourceScopeGroupURL('owners'), { fixture: 'netobserv/flow_metrics_gOwners.json' })
        cy.get(topologySelectors.nGroups).should('have.length', 14)
    })

    it("(OCP-53591, memodi, Network_Observability) should verify group expand/collapse", function () {
        cy.get(topologySelectors.groupToggle).click()
        cy.get(topologySelectors.groupLayer + ' > ' + topologySelectors.group).each((node, index) => {
            cy.wrap(node).should('not.have.descendants', 'g.pf-topology__group')
        })
        cy.get(topologySelectors.groupToggle).click()
        cy.get(topologySelectors.groupLayer + ' > ' + topologySelectors.group).each((node, index) => {
            cy.wrap(node).should('have.descendants', 'g.pf-topology__group')
        })
    })

    it("(OCP-53591, memodi, Network_Observability) should verify edges display/hidden", function () {
        cy.get(topologySelectors.edgeToggle).uncheck()

        // verify labels are also hidden
        cy.get('#edges-tag-switch').should('be.disabled')
        cy.get(topologySelectors.defaultLayer).each((node, index) => {
            cy.wrap(node).should('not.have.descendants', '' + topologySelectors.edge)
        })
        cy.get(topologySelectors.edgeToggle).check()
        cy.get('#edges-tag-switch').should('be.enabled')
        cy.get(topologySelectors.defaultLayer).each((node, index) => {
            cy.wrap(node).should('have.descendants', '' + topologySelectors.edge)
        })
    })

    it("(OCP-53591, memodi, Network_Observability) should verify edges labels can be displayed/hidden", function () {
        cy.byTestID('autocomplete-search').should('exist').type(project + '{enter}')
        topologyPage.selectScopeGroup(null, "none")
        topologyPage.selectScopeGroup("namespace", null)
        cy.get('#reset-view').should('exist').click()

        cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
            cy.wrap(node).should('have.descendants', 'g.pf-topology__edge__tag')
        })
        cy.contains('Display options').should('exist').click()
        cy.get(topologySelectors.labelToggle).uncheck()
        // cy.contains('Display options').should('exist').click()

        cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
            cy.wrap(node).should('not.have.descendants', 'g.pf-topology__edge__tag')
        })
        cy.get(topologySelectors.labelToggle).check()
        cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
            cy.wrap(node).should('have.descendants', 'g.pf-topology__edge__tag')
        })
    })

    it("(OCP-53591, memodi, Network_Observability) should verify badges display/hidden", function () {
        cy.byTestID('autocomplete-search').should('exist').type(project + '{enter}')
        topologyPage.selectScopeGroup(null, "none")
        topologyPage.selectScopeGroup("namespace", null)
        cy.get('#reset-view').should('exist').click()

        cy.contains('Display options').should('exist').click()
        cy.get(topologySelectors.badgeToggle).uncheck()

        cy.get('g.pf-topology__node__label').each((node, index) => {
            cy.wrap(node).should('not.have.descendants', 'g.pf-topology__node__label__badge')
        })
        // not checking the existence of the badge since there may be an 
        // "Unknown" node present with empty badge.
    })

    afterEach("after each tests", function () {
        cy.contains('Display options').should('exist').click()
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })

})
