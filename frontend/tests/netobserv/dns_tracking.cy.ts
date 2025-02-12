import { Operator, project } from "../../views/netobserv"
import { netflowPage, overviewSelectors, querySumSelectors, colSelectors } from "../../views/netflow-page"

describe('(OCP-67087 Network_Observability) DNSTracking test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "DNSTracking")
    })

    beforeEach('any DNSTracking test', function () {
        netflowPage.visit()
    })

    it("(OCP-67087, aramesha, Network_Observability) Verify DNSTracking panels and Query Summary", function () {
        // verify default DNSTracking panels are visible
        cy.checkPanel(overviewSelectors.defaultDNSTrackingPanels)
        cy.checkPanelsNum(5);

        // open panels modal and verify all relevant panels are listed
        cy.openPanelsModal()
        cy.checkPopupItems(overviewSelectors.panelsModal, overviewSelectors.manageDNSTrackingPanelsList);

        // select all panels and verify they are rendered
        cy.get(overviewSelectors.panelsModal).contains('Select all').click();
        cy.get(overviewSelectors.panelsModal).contains('Save').click();
        netflowPage.waitForLokiQuery()
        cy.checkPanelsNum(10);

        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.allDNSTrackingPanels)

        // restore default panels and verify they are visible
        cy.openPanelsModal();
        cy.byTestID(overviewSelectors.resetDefault).click().byTestID(overviewSelectors.save).click()
        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.defaultDNSTrackingPanels)
        cy.checkPanelsNum(5);

        // verify Query Summary stats for DNSTracking
        cy.get(querySumSelectors.dnsAvg).should('exist').then(DNSAvg => {
            cy.checkQuerySummary(DNSAvg)
        })
    })

    it("(OCP-67087, aramesha) Validate DNSTracking columns", function () {
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')
        netflowPage.stopAutoRefresh()

        // verify default DNS columns: DNS Latency and DNS Response Code
        cy.byTestID('table-composable').should('exist').within(() => {
            cy.get(colSelectors.DNSLatency).should('exist')
            cy.get(colSelectors.DNSResponseCode).should('exist')
        })

        // select DNS Id and DNS Error columns
        cy.openColumnsModal().then(col => {
            cy.get(colSelectors.columnsModal).should('be.visible')
            cy.get('#DNSId').check()
            cy.get('#DNSErrNo').check()
            cy.byTestID(colSelectors.save).click()
        })
        cy.reload()

        // verify they are visible in table view
        cy.byTestID('table-composable').should('exist').within(() => {
            cy.get(colSelectors.DNSId).should('exist')
            cy.get(colSelectors.DNSError).should('exist')
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("Delete flowcollector and DNS pods", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
