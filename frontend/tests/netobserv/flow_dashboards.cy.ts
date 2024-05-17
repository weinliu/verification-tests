import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { dashboard } from "views/dashboards-page"

const overviewPanels = [
    "total-egress-traffic-chart",
    "total-ingress-traffic-chart",
    "infra-egress-traffic-chart",
    "apps-egress-traffic-chart",
    "infra-ingress-traffic-chart",
    "apps-ingress-traffic-chart",
    "infra-egress-traffic-chart",
    "apps-egress-traffic-chart"
]

const trafficRatesPanels = [
    "top-egress-traffic-per-node-chart",
    "top-ingress-traffic-per-node-chart",
    "top-egress-traffic-per-infra-namespace-chart",
    "top-ingress-traffic-per-infra-namespace-chart",
    "top-egress-traffic-per-infra-workload-chart",
    "top-ingress-traffic-per-infra-workload-chart"
]

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
