import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage } from "../../views/netflow-page"

describe('(OCP-74049, OCP-73875 Network_Observability) Prometheus datasource only', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project, "LokiDisabled")
    })

    it('(OCP-74049, aramesha, Network_Observability), prom dataSource only', { tags: ['Network_Observability'] }, function () {
        netflowPage.visit()
        // verify overview tab
        cy.get('li.overviewTabButton').should('exist').click()
        cy.wait(2000)
        cy.get('#overview-flex').should('not.be.empty')

        // verify only prom and auto dataSource is enabled in query options
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
        cy.get('#query-options-dropdown').click();
        cy.get('#dataSource-loki').should('be.disabled')
        cy.get('#dataSource-prom').should('not.be.disabled')
        cy.get('#dataSource-auto').should('not.be.disabled')
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();

        // verify netflow traffic page is disabled
        cy.get('li.tableTabButton > button').should('exist').should('have.class', 'pf-m-aria-disabled')

        // verify topology view
        cy.get('li.topologyTabButton').should('exist').click()
        cy.wait(1000)
        cy.get('#drawer').should('not.be.empty')

        // verify resource scop is not observed with prom dataSource
        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('scope-dropdown').click()
            cy.byTestID('resource').should('not.exist')
        })
    })
    after("after all tests are done", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
