import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { dashboard, graphSelector, appsInfra } from "views/dashboards-page"

describe('Network_Observability flow dashboards tests', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project, "AllMetrics")
    })

    it("(OCP-63790, memodi, Network_Observability), should have flow-based dashboards", function () {
        // navigate to 'NetObserv' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

        // verify 'Byte rate received per namespace' panel exists and is populated
        // this panel should appear with the default flowcollector metrics
        cy.byLegacyTestID('panel-byte-rate-received-per-namespace').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Byte rate received per workload' panel exists and is populated
        // this panel should appear with the default flowcollector metrics
        cy.byLegacyTestID('panel-byte-rate-received-per-workload').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Byte rate sent per node' panel exists and is populated
        // this panel should appear with the 'node_egress_bytes_total' metric
        cy.get('[data-test-id="panel-byte-rate-sent-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Packet rate received per node' panel exists and is populated
        // this panel should appear with the 'node_ingress_packets_total' metric
        cy.get('[data-test-id="panel-packet-rate-sent-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Packet rate sent per node' panel exists and is populated
        // this panel should appear with the 'node_egress_packets_total' metric
        cy.get('[data-test-id="panel-packet-rate-received-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'Byte rate send per namespace' panel exists and is populated
        // this panel should appear with the 'namespace_egress_bytes_total' metric
        cy.byLegacyTestID('panel-byte-rate-sent-per-namespace').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet rate sent per namespace' panel exists and is populated
        // this panel should appear with the 'namespace_egress_packets_total' metric
        cy.byLegacyTestID('panel-packet-rate-sent-per-namespace').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet rate received per namespace' panel exists and is populated
        // this panel should appear with the 'namespace_ingress_packets_total' metric
        cy.byLegacyTestID('panel-packet-rate-received-per-namespace').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Byte rate sent per workload' panel exists and is populated
        // this panel should appear with the 'workload_egress_bytes_total' metric
        cy.byLegacyTestID('panel-byte-rate-sent-per-workload').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet rate sent per workload' panel exists and is populated
        // this panel should appear with the 'workload_egress_packets_total' metric
        cy.byLegacyTestID('panel-packet-rate-sent-per-workload').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Packet rate received per workload' panel exists and is populated
        // this panel should appear with the 'workload_ingress_packets_total' metric
        cy.byLegacyTestID('panel-packet-rate-received-per-workload').should('exist').within(topBytes => {
            cy.checkDashboards(appsInfra)
        })
    })

    after("delete flowcollector and NetObs Operator", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
