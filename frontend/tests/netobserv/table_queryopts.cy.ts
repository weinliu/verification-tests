import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"

function getTableLimitURL(limit: string): string {
    return `**/netflow-traffic**limit=${limit}`
}

describe('(OCP-50532, OCP-50531, OCP-50530, OCP-59408 Network_Observability) Netflow Table Query Options', { tags: ['Network_Observability'] }, function () {

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

    it("(OCP-50532, aramesha, Network_Observability) should verify Query Options dropdown", { tags: ['e2e', 'admin', '@smoke'] }, function () {
        // toggle between the page limits
        cy.changeQueryOption('500')
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getTableLimitURL('500'), {
            fixture: 'netobserv/netflow_table_500.json'
        }).as('matchedUrl')

        cy.changeQueryOption('100')
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getTableLimitURL('100'), {
            fixture: 'netobserv/netflow_table_100.json'
        }).as('matchedUrl')

        cy.changeQueryOption('50')
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getTableLimitURL('50'), {
            fixture: 'netobserv/netflow_table_50.json'
        }).as('matchedUrl')
    })

    it("(OCP-50532, memodi, Network_Observability) should validate query summary panel", { tags: ['e2e', 'admin', '@smoke'] }, function () {
        let warningExists = false
        cy.get(querySumSelectors.queryStatsPanel).should('exist').then(qrySum => {
            if (Cypress.$(querySumSelectors.queryStatsPanel + ' svg.query-summary-warning').length > 0) {
                warningExists = true
            }
        })

        cy.get(querySumSelectors.flowsCount).should('exist').then(flowsCnt => {
            let nflows = 0
            if (warningExists) {
                nflows = Number(flowsCnt.text().split('+ Flows')[0])
            }
            else {
                nflows = Number(flowsCnt.text().split(' ')[0])
            }
            cy.wait(10)
            expect(nflows).to.be.greaterThan(0)
        })

        cy.get(querySumSelectors.bytesCount).should('exist').then(bytesCnt => {
            let nbytes = 0
            if (warningExists) {
                nbytes = Number(bytesCnt.text().split('+ ')[0])
            }
            else {
                nbytes = Number(bytesCnt.text().split(' ')[0])
            }
            expect(nbytes).to.be.greaterThan(0)
        })

        cy.get(querySumSelectors.packetsCount).should('exist').then(pktsCnt => {
            let npkts = 0
            if (warningExists) {
                let npktsStr = pktsCnt.text().split('+ ')[0]
                if (npktsStr.includes('k')) {
                    npkts = Number(npktsStr.split('k')[0])
                }
                else {
                    npkts = Number(npktsStr)
                }
            }
            else {
                npkts = Number(pktsCnt.text().split(' ')[0])
            }
            expect(npkts).to.be.greaterThan(0)
        })
        cy.get('#query-summary-toggle').should('exist').click()
        cy.get('#summaryPanel').should('be.visible')

        cy.contains('Results').should('exist')
        cy.contains('Average time').should('exist')
        cy.contains('Duration').should('exist')
        cy.contains('Collection latency').should('exist')
        cy.contains('Cardinality').should('exist')
        cy.contains('Configuration').should('exist')
        cy.contains('Sampling').should('exist')
        cy.contains('Version').should('exist')
        cy.contains('Number').should('exist')
        cy.contains('Date').should('exist')
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
