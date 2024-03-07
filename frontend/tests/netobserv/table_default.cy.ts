import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, colSelectors } from "../../views/netflow-page"

describe('(OCP-50532, OCP-50531, OCP-50530, OCP-59408 Network_Observability) Netflow Table default view tests', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project)
    })

    beforeEach('any netflow table test', function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')
    })

    it("(OCP-71787, aramesha, Network_Observability)should verify conversation tracking is disabled by default", function () {
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
        cy.get('#query-options-dropdown').click();
        cy.get('#recordType-allConnections').should('be.disabled')
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
    })

    it("(OCP-66141, aramesha, Network_Observability)should verify packet drop filters are disabled by default", function () {
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
        cy.get('#query-options-dropdown').click();
        cy.get('#packet-loss-dropped').should('be.disabled')
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
    })

    it("(OCP-68125, aramesha, Network_Observability)should verify DSCP column is enbaled by default", function () {
        cy.byTestID('table-composable').should('exist').within(() => {
            cy.get(colSelectors.DSCP).should('exist')
        })

        // filter on DSCP values
        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        // verify drop TCP state filter
        cy.byTestID('group-2-toggle').click().should('be.visible')
        cy.byTestID('dscp').click()
        cy.byTestID('autocomplete-search').type('0' + '{enter}')
        cy.get('#filters div.custom-chip > p').should('contain.text', 'Standard')

        // Verify DSCP value is Standard for all rows
        cy.get('[data-test-td-column-id=Dscp]').each((td) => {
            expect(td).attr("data-test-td-value").to.contain(0)
            cy.get('[data-test-td-column-id=Dscp] > div > div > span').should('contain.text', 'Standard')
        })
    })
    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
