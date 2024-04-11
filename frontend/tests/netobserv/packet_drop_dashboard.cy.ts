
import { dashboard, graphSelector, appsInfra } from "views/dashboards-page"
import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"

describe('(OCP-66141 Network_Observability) PacketDrop dashboards test', { tags: ['Network_Observability'] }, function () {
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

    it("(OCP-66141, aramesha, Network_Observability) Validate PacketDrop edge labels and Query Summary stats", { tags: ['e2e', 'admin'] }, function () {
        netflowPage.visit()
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

        // update metricType to Dropped bytes
        cy.get('#metricType-dropdown').click()
        cy.get('#PktDropBytes').click()

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
        cy.get('#PktDropPackets').click()

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
        netflowPage.resetClearFilters()
    })

    it("(OCP-66141, aramesha, Network_Observability) Validate packetDrop dashboards", { tags: ['e2e', 'admin'] }, function () {
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
    })
})
