import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, overviewSelectors } from "../../views/netflow-page"

function getPacketDropURL(drop: string): string {
    return `**/netflow-traffic**packetLoss=${drop}`
}

describe('(OCP-66141 Network_Observability) PacketDrop test', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project, "PacketDrop")
    })

    beforeEach('any packetDrop test', function () {
        netflowPage.visit()
    })

    it("(OCP-66141, aramesha, Network_Observability) verify packetDrop panels", { tags: ['e2e', 'admin'] }, function () {
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
        cy.byTestID('view-options-button').click()
        cy.get(overviewSelectors.mPanels).click().byTestID(overviewSelectors.resetDefault).click().byTestID(overviewSelectors.save).click()
        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.defaultPacketDropPanels)
        cy.checkPanelsNum(6);
    })

    it("(OCP-66141, aramesha, Network_Observability) Verify packetDrop Query Options filters", { tags: ['e2e', 'admin'] }, function () {
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

    it("(OCP-66141, aramesha, Network_Observability) Validate packetDrop filters", { tags: ['e2e', 'admin'] }, function () {
        netflowPage.stopAutoRefresh()

        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        // verify drop TCP state filter
        cy.byTestID('group-2-toggle').click().should('be.visible')
        cy.byTestID('pkt_drop_state').click()
        cy.byTestID('autocomplete-search').type('INVALID_STATE' + '{enter}')
        cy.get('#filters div.custom-chip > p').should('contain.text', 'INVALID_STATE')
        cy.get('#filters div.custom-chip-group > p').should('contain.text', 'Packet drop TCP state')

        // verify dropped state panel has only INVALID_STATE
        cy.get('#state_dropped_packet_rates').within(() => {
            cy.get('#chart-legend-5-ChartLabel-0').should('contain.text', 'TCP_INVALID_STATE')
            cy.get('#chart-legend-5-ChartLabel-1').should('contain.text', 'Total')
            cy.get('#chart-legend-5-ChartLabel-2').should('not.exist')
        })

        // verify drop latest cause filter
        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('pkt_drop_cause').click()
        cy.byTestID('autocomplete-search').type('NO_SOCKET' + '{enter}')
        cy.get('#filters div.custom-chip > p').should('contain.text', 'NO_SOCKET')
        cy.get('#filters div.custom-chip-group > p').should('contain.text', 'Packet drop latest cause')

        // verify dropped cause panel has only NO_SOCKET
        cy.get('#cause_dropped_packet_rates').within(() => {
            cy.get('#chart-legend-5-ChartLabel-0').should('contain.text', 'SKB_DROP_REASON_NO_SOCKET')
            cy.get('#chart-legend-5-ChartLabel-1').children().should('contain.text', 'Total')
            cy.get('#chart-legend-5-ChartLabel-2').should('not.exist')
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
