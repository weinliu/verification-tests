import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"

describe('(OCP-67782 Network_Observability) Large volume downloads counters test', { tags: ['Network_Observability'] }, function () {

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

    describe("Large volume downloads test", function () {
        beforeEach('any large volume downloads test', function () {
            netflowPage.visit()
            cy.get('#tabs-container li:nth-child(2)').click()
            cy.byTestID("table-composable").should('exist')
        })

        it("(OCP-67782, aramesha, Network_Observability) should verify large volume download counter", function () {
            // Filter on SrcPort 443, DstNamespace test-client and DstName client
            // create test server and client pods
            cy.adminCLI('oc create -f ./fixtures/netobserv/test-client-large-download.yaml')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('src_port').click()
            cy.byTestID('autocomplete-search').type('443' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('group-1-toggle').click().should('be.visible')
            cy.byTestID('dst_name').click()
            cy.get('#search').type('client' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('dst_namespace').click()
            cy.byTestID('autocomplete-search').type('test-client')
            cy.get('#search-button').click()

            netflowPage.waitForLokiQuery()

            // wait for download to finish
            cy.wait(2000)

            // get bytesCount from query summary panel
            let warningExists = false
            cy.get(querySumSelectors.queryStatsPanel).should('exist').then(qrySum => {
                if (Cypress.$(querySumSelectors.queryStatsPanel + ' svg.query-summary-warning').length > 0) {
                    warningExists = true
                }
            })

            cy.get(querySumSelectors.bytesCount).should('exist').then(bytesCnt => {
                let nbytes = 0
                if (warningExists) {
                    nbytes = Number(bytesCnt.text().split('+ ')[0])
                }
                else {
                    nbytes = Number(bytesCnt.text().split(' ')[0])
                }
                // curl total = 291M for "Fedora-Cloud-Base-Vagrant-30-1.2.x86_64.vagrant-libvirt.box" image
                expect(nbytes).to.be.closeTo(291, 20, "Expected number of bytes is wrong")
            })
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI('oc delete -f ./fixtures/netobserv/test-client-large-download.yaml')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
