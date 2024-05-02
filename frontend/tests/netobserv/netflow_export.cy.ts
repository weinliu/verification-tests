import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, exportSelectors } from "../../views/netflow-page"

describe('(OCP-72610 Network_Observability) Export automation', { tags: ['Network_Observability'] }, function () {

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

    beforeEach('any export test', function () {
        netflowPage.visit()
    })

    it("(OCP-72610, aramesha, Network_Observability) should validate exporting panels", function () {
        // Export all overview panels
        cy.get('li.overviewTabButton').should('exist').click()
        cy.byTestID("show-view-options-button").should('exist').click()
        netflowPage.stopAutoRefresh()
        cy.byTestID('view-options-button').should('exist').click()
        cy.get(exportSelectors.overviewExport).should('exist').click()
        cy.readFile('cypress/downloads/overview_page.png')

        // Export only Top 5 average bytes rates panel
        cy.get(exportSelectors.avgBytesRatesDropdown).should('exist').click()
        cy.contains("Export panel").should('exist').click()
        cy.readFile('cypress/downloads/overview_panel_top_avg_byte_rates.png')
        cy.exec('rm cypress/downloads/overview_page.png')
        cy.exec('rm cypress/downloads/overview_panel_top_avg_byte_rates.png')
    })

    it("(OCP-72610, aramesha, Network_Observability) should validate exporting table view", function () {
        cy.get('li.tableTabButton').should('exist').click()
        netflowPage.stopAutoRefresh()
        netflowPage.selectSourceNS(project)
        cy.byTestID("table-composable").should('exist')
        cy.byTestID("show-view-options-button").should('exist').click()
        cy.byTestID('view-options-button').should('exist').click()
        cy.get(exportSelectors.tableExport).should('exist').click()
        cy.get(exportSelectors.exportButton).should('exist').then((exportbtn) => {
            cy.wrap(exportbtn).click()
            // wait for download to complete
            cy.wait(3000)
            // get the CSV file name
            cy.exec("ls cypress/downloads").then((response) => {
                // rename CSV file to export_table.csv
                cy.wrap(response.stdout).should('not.be.empty')
                cy.exec(`mv cypress/downloads/${response.stdout} cypress/downloads/export_table.csv`)
                cy.readFile('cypress/downloads/export_table.csv')
            })
            cy.exec('rm cypress/downloads/export_table.csv')
        })
    })

    it("(OCP-72610, aramesha, Network_Observability) should validate exporting topology view", function () {
        cy.get('li.topologyTabButton').should('exist').click()
        netflowPage.selectSourceNS(project)
        netflowPage.stopAutoRefresh()
        cy.get('#drawer').should('not.be.empty')
        cy.byTestID("show-view-options-button").should('exist').click()
        cy.byTestID('view-options-button').should('exist').click()
        cy.get(exportSelectors.topologyExport).should('exist').click()
        cy.readFile('cypress/downloads/topology.png').then(() => {
            cy.exec('rm cypress/downloads/topology.png')
        })

    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
