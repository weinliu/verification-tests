import { Operator, project } from "../../views/netobserv"
import { dashboard } from "views/dashboards-page"

const overviewPanels = [
    "total-egress-traffic-chart",
    "total-ingress-traffic-chart",
    "infra-egress-traffic-chart",
    "infra-ingress-traffic-chart"
]

const trafficRatesPanels = [
    "top-egress-traffic-per-node-(bps)-chart",
    "top-ingress-traffic-per-node-(bps)-chart",
    "top-egress-traffic-per-node-(pps)-chart",
    "top-ingress-traffic-per-node-(pps)-chart",
    "top-egress-traffic-per-infra-namespace-(bps)-chart",
    "top-ingress-traffic-per-infra-namespace-(bps)-chart",
    "top-egress-traffic-per-infra-namespace-(pps)-chart",
    "top-ingress-traffic-per-infra-namespace-(pps)-chart",
    "top-egress-traffic-per-infra-workload-(bps)-chart",
    "top-ingress-traffic-per-infra-workload-(bps)-chart",
    "top-egress-traffic-per-infra-workload-(pps)-chart",
    "top-ingress-traffic-per-infra-workload-(pps)-chart"
]

describe('Network_Observability flow dashboards tests', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "AllMetrics")
    })

    it("(OCP-63790, memodi, Network_Observability), should have flow-based dashboards", function () {
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        // verify that overview panels panels exist and are populated
        cy.checkDashboards(overviewPanels)

        // verify that Traffic Rates panels panels exist and are populated
        cy.checkDashboards(trafficRatesPanels)
    })

    after("delete flowcollector and NetObs Operator", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
