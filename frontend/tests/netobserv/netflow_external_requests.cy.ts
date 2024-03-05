import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage } from "../../views/netflow-page"

describe('(OCP-67615 Network_Observability) Return traffic for external traffic test', { tags: ['Network_Observability'] }, function () {

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

        // deploy test pod
        cy.adminCLI('oc create -f ./fixtures/netobserv/test-pod.yaml')
    })

    describe("Return traffic for external traffic test", function () {
        beforeEach('any return traffic for external traffic test', function () {
            netflowPage.visit()
            cy.get('#tabs-container li:nth-child(2)').click()
            cy.byTestID("table-composable").should('exist')
        })

        it("(OCP-67615, aramesha, Network_Observability) should verify return traffic for external traffic", function () {
            // filter on SrcName test, DstIP 52.200.142.250
            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('src_name').click()
            cy.get('#search').type('test' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('group-1-toggle').click().should('be.visible')
            cy.byTestID('dst_address').click()
            cy.get('#search').type('52.200.142.250' + '{enter}')

            netflowPage.waitForLokiQuery()

            // validate rows count=1, DstNamespace and DstName is n/a
            cy.byTestID('table-composable').each((td) => {
                expect(td).attr("data-test-rows-count").to.contain(1)
            })
            cy.get('[data-test-td-column-id=DstK8S_Name]').each((td) => {
                expect(td).attr("data-test-td-value").to.be.empty
            })
            cy.get('[data-test-td-column-id=DstK8S_Namespace]').each((td) => {
                expect(td).attr("data-test-td-value").to.be.empty
            })

            // click on Back-on-Forth button
            cy.get('#chips-more-options-dropdown').should('exist').click().then(moreOpts => {
                cy.contains("One way").should('exist').click()
            })

            // validate rows count=2
            cy.byTestID('table-composable').each((td) => {
                expect(td).attr("data-test-rows-count").to.contain(2)
            })
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI('oc delete -f ./fixtures/netobserv/test-pod.yaml')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
