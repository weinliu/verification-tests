import { Operator, project } from "../../views/netobserv"
import { netflowPage, overviewSelectors } from "../../views/netflow-page"

function getPacketDropURL(drop: string): string {
    return `**/netflow-traffic**packetLoss=${drop}`
}

describe('(OCP-66141 Network_Observability) PacketDrop test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "PacketDrop")
    })

    beforeEach('any packetDrop test', function () {
        netflowPage.visit()
    })

    it("(OCP-66141, aramesha, Network_Observability) Verify packetDrop panels", function () {
        netflowPage.stopAutoRefresh()

        // verify default PacketDrop panels are visible
        cy.checkPanel(overviewSelectors.defaultPacketDropPanels)
        cy.checkPanelsNum(6);

        // open panels modal and verify all relevant panels are listed
        cy.openPanelsModal();
        cy.checkPopupItems(overviewSelectors.panelsModal, overviewSelectors.managePacketDropPanelsList);

        // select all panels and verify they are rendered
        cy.get(overviewSelectors.panelsModal).contains('Select all').click();
        cy.get(overviewSelectors.panelsModal).contains('Save').click();
        netflowPage.waitForLokiQuery()
        cy.checkPanelsNum(10);

        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.allPacketDropPanels)

        // restore default panels and verify they are visible
        cy.openPanelsModal().byTestID(overviewSelectors.resetDefault).click().byTestID(overviewSelectors.save).click()
        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.defaultPacketDropPanels)
        cy.checkPanelsNum(6);
    })

    it("(OCP-66141, aramesha, Network_Observability) Verify packetDrop Query Options filters", function () {
        netflowPage.stopAutoRefresh()

        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')

        // toggle between drops filter
        cy.changeQueryOption('Fully dropped');
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getPacketDropURL('dropped'), {
            fixture: 'netobserv/flow_records_fully_dropped.json'
        }).as('matchedUrl')

        cy.changeQueryOption('Without drops')
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getPacketDropURL('hasDrops'), {
            fixture: 'netobserv/flow_records_without_drops.json'
        }).as('matchedUrl')

        cy.changeQueryOption('Containing drops')
        netflowPage.waitForLokiQuery()
        cy.intercept('GET', getPacketDropURL('sent'), {
            fixture: 'netobserv/flow_records_containing_drops.json'
        }).as('matchedUrl')
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
