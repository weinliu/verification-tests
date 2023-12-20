import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, overviewSelectors, querySumSelectors} from "../../views/netflow-page"
import { dashboard, graphSelector, appsInfra} from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "Dropped bytes",
    "Dropped packets"
]

function getPacketDropURL(drop: string): string {
    return `**/netflow-traffic**packetLoss=${drop}`
}

describe('(OCP-66141 NETOBSERV) PacketDrop test', { tags: ['NETOBSERV'] }, function () {

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

    describe("PacketDrop features", function () {
        beforeEach('any packetDrop test', function () {
            netflowPage.visit()
        })

        it("(OCP-66141, aramesha) verify packetDrop panels", { tags: ['e2e', 'admin'] }, function () {
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

        it("(OCP-66141, aramesha) should validate query summary panel with packetDrop enabled", function () {
            cy.get('#query-summary-toggle').should('exist').click()
            cy.get('#summaryPanel').should('be.visible')

            cy.get(querySumSelectors.droppedBytesCount).should('exist').then(droppedBytesCnt => {
                cy.checkQuerySummary(droppedBytesCnt)
            })

            cy.get(querySumSelectors.droppedBpsCount).should('exist').then(droppedBpsCnt => {
                cy.checkQuerySummary(droppedBpsCnt)
            })

            cy.get(querySumSelectors.droppedPacketsCount).should('exist').then(droppedPacketsCnt => {
                cy.checkQuerySummary(droppedPacketsCnt)
            })
        })

        it("(OCP-66141, aramesha) Verify packetDrop Query Options filters", { tags: ['e2e', 'admin'] }, function () {
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

        it("(OCP-66141, aramesha) Validate packetDrop filters", { tags: ['e2e', 'admin'] }, function () {
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

        it("(OCP-66141, aramesha) Validate PacketDrop edge labels and Query Summary stats", { tags: ['e2e', 'admin'] }, function () {
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

            cy.byTestID('metricType').should('exist').click()
            cy.get('#metricType > ul > li').should('have.length', 4).each((item, index) => {
                cy.wrap(item).should('contain.text', metricType[index])
            })

            // update metricType to Dropped bytes
            cy.get('#droppedBytes').click()

            // verify Query Summary stats for Dropped Bytes metric
            cy.get(querySumSelectors.droppedBytesCount).should('exist').then(droppedBytesCnt => {
                cy.checkQuerySummary(droppedBytesCnt)
            })

            cy.get(querySumSelectors.droppedBpsCount).should('exist').then(droppedBpsCnt => {
                cy.checkQuerySummary(droppedBpsCnt)
            })

            cy.get(querySumSelectors.droppedPacketsCount).should('exist').then(droppedPacketsCnt => {
                cy.checkQuerySummary(droppedPacketsCnt)
            })

            // update metricType to Dropped packets
            cy.byTestID('metricType').should('exist').click()
            cy.get('#droppedPackets').click()

            // verify Query Summary stats for Dropped Bytes metric
            cy.get(querySumSelectors.droppedBytesCount).should('exist').then(droppedBytesCnt => {
                cy.checkQuerySummary(droppedBytesCnt)
            })

            cy.get(querySumSelectors.droppedBpsCount).should('exist').then(droppedBpsCnt => {
                cy.checkQuerySummary(droppedBpsCnt)
            })

            cy.get(querySumSelectors.droppedPacketsCount).should('exist').then(droppedPacketsCnt => {
                cy.checkQuerySummary(droppedPacketsCnt)
            })
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })
})

describe('(OCP-66141 NETOBSERV) PacketDrop dashboards test', { tags: ['NETOBSERV'] }, function () {
    it("(OCP-66141, aramesha) Validate packetDrop dashboards", { tags: ['e2e', 'admin'] }, function () {
        // navigate to 'NetObserv' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

        // verify 'Byte drop rate per node' panel
        // panel should appear with the flowcollector metric 'node_drop_bytes_total'
        cy.get('[data-test-id="panel-byte-drop-rate-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Byte drop rate per node' panel
        // panel should appear with the flowcollector metric 'node_drop_packets_total'
        cy.get('[data-test-id="panel-packet-drop-rate-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Byte drop rate per namespace' panel
        // panel should appear with the flowcollector metric 'namespace_drop_bytes_total'
        cy.byLegacyTestID('panel-byte-drop-rate-per-namespace').should('exist').within(byteDropRate => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet drop rate per namespace' panel
        // panel should appear with the flowcollector metric 'namespace_drop_packets_total'
        cy.byLegacyTestID('panel-packet-drop-rate-per-namespace').should('exist').within(packetDropRate => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Byte drop rate per workload' panel
        // panel should appear with the flowcollector metric 'workload_drop_bytes_total'
        cy.byLegacyTestID('panel-byte-drop-rate-per-workload').should('exist').within(byteDropRate => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet drop rate per workload' panel
        // panel should appear with the flowcollector metric 'workload_drop_packets_total'
        cy.byLegacyTestID('panel-packet-drop-rate-per-workload').should('exist').within(packetDropRate => {
            cy.checkDashboards(appsInfra)
        })
    })

    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogout()
    })
})
