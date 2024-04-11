import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { dashboard, graphSelector, appsInfra } from "views/dashboards-page"

const cpuMemory = [
    "cpu-usage-chart",
    "memory-usage-chart"
]
// skipping until we figure out how to run dashboard tests faster
describe.skip('Network_Observability dashboards tests', { tags: ['Network_Observability'] }, function () {

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

    it('(OCP-61893, memodi, Network_Observability), should have default health dashboards', { tags: ['e2e', 'admin', '@smoke'] }, function () {
        // navigate to 'NetObserv / Health' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-health")

        // verify that 'Flows' and 'Flows Overhead' panels exist and are populated
        // this panel should appear with the default flowcollector metrics
        var flowPanels: string[] = ['rates-chart', 'percentage-of-flows-generated-by-netobserv-own-traffic-chart']
        cy.checkDashboards(flowPanels)

        // verify 'Top flow rates per source and destination namespaces' panel exists and is populated
        // this panel should appear with the default flowcollector metrics
        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-namespaces').should('exist').within(topflow => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Agents' CPU and memory usage panels exists and are populated
        cy.byLegacyTestID('panel-agents').should('exist').within(agent => {
            cy.checkDashboards(cpuMemory)
        })

        // verify 'Processor' CPU and memory usage panels exists and are populated
        cy.byLegacyTestID('panel-processor').should('exist').within(processor => {
            cy.checkDashboards(cpuMemory)
        })

        // verify 'Operator' reconciliation rate panel exists and is populated
        cy.get('[data-test="operator-reconciliation-rate-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // check that panels below do NOT appear
        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-workloads').should('not.exist')
        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-nodes').should('not.exist')
    })

    it('(OCP-61893, nweinber, Network_Observability), should have health dashboards from additional metrics', function () {
        // recreate flowcollector with additional metrics and go back to the dashboard page
        Operator.createFlowcollector(project, "AllMetrics")
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-health")

        // verify 'Top flow rates per source and destination workloads' panel exists and is populated
        // this panel should appear with the flowcollector metric 'workload_flows_total'
        cy.byLegacyTestID('panel-top-flow-rates-per-source-and-destination-workloads').should('exist').within(topflow => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'Top flow rates per source and destination nodes' panel exists and is populated
        // this panel should appear with the flowcollector metric 'node_flows_total'
        cy.byTestID("-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    })

    it("(OCP-63790, memodi, Network_Observability), should have default flow-based dashboards", { tags: ['e2e', 'admin', '@smoke'] }, function () {
        // navigate to 'NetObserv' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

        // verify 'Byte rate received per node' panel exists and is populated
        // this panel should appear with the default flowcollector metrics
        cy.byTestID("-chart").find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

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
    })

    it("(OCP-63790, nweinber, Network_Observability), should have flow-based dashboards from additional metrics", function () {
        // note flowcollector with additional metrics should already exist here
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

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
