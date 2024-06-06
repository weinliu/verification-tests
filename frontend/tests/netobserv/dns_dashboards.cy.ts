import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, querySumSelectors } from "../../views/netflow-page"
import { dashboard } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "DNS latencies"
]

const DNSPanels = [
    // below 3 panels should appear with the 'node_dns_latency_seconds' metric
    "top-p50-dns-latency-per-node-(ms)-chart",
    "top-p99-dns-latency-per-node-(ms)-chart",
    "dns-error-rate-per-node-chart",
    // below 3 panels should appear with the 'namespace_dns_latency_seconds' metric
    "top-p50-dns-latency-per-infra-namespace-(ms)-chart",
    "top-p99-dns-latency-per-infra-namespace-(ms)-chart",
    "dns-error-rate-per-infra-namespace-chart",
    // below 3 panels should appear with the 'workload_dns_latency_seconds' metric
    "top-p50-dns-latency-per-infra-workload-(ms)-chart",
    "top-p99-dns-latency-per-infra-workload-(ms)-chart",
    "dns-error-rate-per-infra-workload-chart"
]

describe('(OCP-67087 Network_Observability) DNSTracking test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

        // create DNS over TCP and UDP pods
        cy.adminCLI('oc apply -f ./fixtures/netobserv/DNS-pods.yaml')

        // sepcify --env noo_release=upstream to run tests
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
        Operator.createFlowcollector(project, "DNSTracking")
    })

    it("(OCP-67087, aramesha, Network_Observability) Validate DNS over TCP and UDP", function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')

        // filter on SrcPort 53, DstName dnsutils1, DstNamespace dns-traffic and DNSError = 0
        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('src_port').click()
        cy.byTestID('autocomplete-search').type('53' + '{enter}')

        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('group-2-toggle').click().should('be.visible')
        cy.byTestID('dns_errno').click()
        cy.byTestID('autocomplete-search').type('0' + '{enter}')

        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('group-1-toggle').click().should('be.visible')
        cy.byTestID('dst_namespace').click()
        cy.byTestID('autocomplete-search').type('dns-traffic')
        cy.get('#search-button').click()

        cy.get('#filters div.custom-chip > p').should('contain.text', 'dns-traffic')

        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('dst_name').click()
        cy.get('#search').type('dnsutils1' + '{enter}')

        netflowPage.waitForLokiQuery()

        // verify Protocol is TCP and DNS Latencies > 0 for all rows
        cy.get('[data-test-td-column-id=Proto]').each((td) => {
            expect(td).attr("data-test-td-value").to.contain(6)
        })
        cy.get('[data-test-td-column-id=DNSLatency]').each((td) => {
            expect(td).attr("data-test-td-value").to.match(RegExp("^[0-9]*$"))
        })

        cy.get('#filters div:nth-child(4) > button').should('exist').click()

        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('dst_name').click()
        cy.get('#search').type('dnsutils2' + '{enter}')

        netflowPage.waitForLokiQuery()

        // verify Protocol is UDP and DNS Latencies > 0  for all rows
        cy.get('[data-test-td-column-id=Proto]').each((td) => {
            expect(td).attr("data-test-td-value").to.contain(17)
        })
        cy.get('[data-test-td-column-id=DNSLatency]').each((td) => {
            expect(td).attr("data-test-td-value").to.match(RegExp("^[0-9]*$"))
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-67087, aramesha, Network_Observability) Validate DNSLatencies edge label and Query Summary stats", function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(3)').click()
        cy.get('#drawer').should('not.be.empty')

        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('layout-dropdown').click()
            cy.byTestID('Grid').click()
        })

        cy.byTestID('metricType').should('exist').click()
        cy.get('#metricType > ul > li').should('have.length', 3).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })

        cy.get('#DnsLatencyMs').click()
        cy.byTestID("scope-dropdown").click().byTestID("host").click()
        cy.contains('Display options').should('exist').click()

        // validate edge labels shows DNS latency info
        cy.get('#zoom-in').click({ force: true }).click({ force: true }).click({ force: true });

        cy.get('[data-test-id=edge-handler]').should('exist').each((g) => {
            expect(g.text()).to.match(/\d* ms/gm);
        });

        // verify Query Summary stats for DNSTracking
        cy.get(querySumSelectors.dnsAvg).should('exist').then(DNSAvg => {
            cy.checkQuerySummary(DNSAvg)
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-67087, aramesha, Network_Observability) Validate DNSTracking dashboards", function () {
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        cy.checkDashboards(DNSPanels)
    })

    after("Delete flowcollector and DNS pods", function () {
        cy.adminCLI('oc delete -f ./fixtures/netobserv/DNS-pods.yaml')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
