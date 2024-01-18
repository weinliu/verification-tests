import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, overviewSelectors, querySumSelectors, colSelectors } from "../../views/netflow-page"
import { dashboard, graphSelector, appsInfra } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "DNS latencies"
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

    describe("DNSTracking features", function () {
        beforeEach('any DNSTracking test', function () {
            netflowPage.visit()
        })

        it("(OCP-67087, aramesha, Network_Observability) Verify DNSTracking panels and Query Summary", { tags: ['e2e', 'admin'] }, function () {
            // verify default DNSTracking panels are visible
            cy.checkPanel(overviewSelectors.defaultDNSTrackingPanels)
            cy.checkPanelsNum(5);

            // open panels modal and verify all relevant panels are listed
            cy.openPanelsModal();
            cy.checkPopupItems(overviewSelectors.panelsModal, overviewSelectors.manageDNSTrackingPanelsList);

            // select all panels and verify they are rendered
            cy.get(overviewSelectors.panelsModal).contains('Select all').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanelsNum(10);
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.allDNSTrackingPanels)

            // restore default panels and verify they are visible
            cy.byTestID('view-options-button').click()
            cy.get(overviewSelectors.mPanels).click().byTestID(overviewSelectors.resetDefault).click().byTestID(overviewSelectors.save).click()
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.defaultDNSTrackingPanels)
            cy.checkPanelsNum(5);

            // verify Query Summary stats for DNSTracking
            cy.get(querySumSelectors.dnsAvg).should('exist').then(DNSAvg => {
                cy.checkQuerySummary(DNSAvg)
            })
        })

        it("(OCP-67087, aramesha) Validate DNSTracking columns", { tags: ['e2e', 'admin'] }, function () {
            cy.get('#tabs-container li:nth-child(2)').click()
            cy.byTestID("table-composable").should('exist')
            netflowPage.stopAutoRefresh()

            // verify default DNS columns: DNS Latency and DNS Response Code
            cy.byTestID('table-composable').should('exist').within(() => {
                cy.get(colSelectors.DNSLatency).should('exist')
                cy.get(colSelectors.DNSResponseCode).should('exist')
            })

            // select DNS Id and DNS Error columns
            cy.byTestID("show-view-options-button").should('exist').click()
            cy.byTestID('view-options-button').click()
            cy.get(colSelectors.mColumns).click().then(col => {
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

        it("(OCP-67087, aramesha, Network_Observability) Validate DNS over TCP", { tags: ['e2e', 'admin'] }, function () {
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
            cy.byTestID('dst_name').click()
            cy.get('#search').type('dnsutils1' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('dst_namespace').click()
            cy.byTestID('autocomplete-search').type('dns-traffic')
            cy.get('#search-button').click()

            netflowPage.waitForLokiQuery()

            // verify Protocol is TCP and DNS Latencies > 0 for all rows
            cy.get('[data-test-td-column-id=Proto]').each((td) => {
                expect(td).attr("data-test-td-value").to.contain(6)
            })
            cy.get('[data-test-td-column-id=DNSLatency]').each((td) => {
                expect(td).attr("data-test-td-value").to.match(RegExp("^[0-9]*$"))
            })
        })

        it("(OCP-67087, aramesha, Network_Observability) Validate DNS over UDP", { tags: ['e2e', 'admin'] }, function () {
            cy.get('#tabs-container li:nth-child(2)').click()
            cy.byTestID("table-composable").should('exist')

            // filter on SrcPort 53, DstName dnsutils2, DstNamespace dns-traffic and DNSError = 0
            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('src_port').click()
            cy.byTestID('autocomplete-search').type('53' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('group-2-toggle').click().should('be.visible')
            cy.byTestID('dns_errno').click()
            cy.byTestID('autocomplete-search').type('0' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('group-1-toggle').click().should('be.visible')
            cy.byTestID('dst_name').click()
            cy.get('#search').type('dnsutils2' + '{enter}')

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
            cy.byTestID('dst_namespace').click()
            cy.byTestID('autocomplete-search').type('dns-traffic')
            cy.get('#search-button').click()

            netflowPage.waitForLokiQuery()

            // verify Protocol is UDP and DNS Latencies > 0  for all rows
            cy.get('[data-test-td-column-id=Proto]').each((td) => {
                expect(td).attr("data-test-td-value").to.contain(17)
            })
            cy.get('[data-test-td-column-id=DNSLatency]').each((td) => {
                expect(td).attr("data-test-td-value").to.match(RegExp("^[0-9]*$"))
            })
        })

        it("(OCP-67087, aramesha, Network_Observability) Validate DNSLatencies edge label and Query Summary stats", { tags: ['e2e', 'admin'] }, function () {
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
            cy.get('#metricType > ul > li').should('have.length', 3).each((item, index) => {
                cy.wrap(item).should('contain.text', metricType[index])
            })

            cy.get('#dnsLatencies').click()
            cy.contains('Display options').should('exist').click()

            // validate ege labels shows DNS latency info
            cy.get('#zoom-in').click().click().click();

            cy.get('[data-test-id=edge-handler]').each((g) => {
                expect(g.text()).to.match(/\d* ms/gm);
            });

            // verify Query Summary stats for DNSTracking
            cy.get(querySumSelectors.dnsAvg).should('exist').then(DNSAvg => {
                cy.checkQuerySummary(DNSAvg)
            })
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })
})

describe('(OCP-67087 Network_Observability) DNSTracking dashboards test', { tags: ['Network_Observability'] }, function () {
    it("(OCP-67087, aramesha) Validate DNSTracking dashboards", { tags: ['e2e', 'admin'] }, function () {
        // navigate to 'NetObserv' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("grafana-dashboard-netobserv-flow-metrics")

        // verify 'DNS latency per node (milliseconds - p99 and p50)' panel
        // below 2 panels should appear with the flowcollector metric 'node_dns_latency_seconds'
        cy.get('[data-test-id="panel-dns-latency-per-node-milliseconds-p-99-and-p-50"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'DNS request rate per code and per node' panel
        cy.get('[data-test-id="panel-dns-request-rate-per-code-and-per-node"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        // verify 'DNS latency per namespace (milliseconds - p99 and p50)' panel
        // below 2 panels should appear with the flowcollector metric 'namespace_dns_latency_seconds'
        cy.byLegacyTestID('panel-dns-latency-per-namespace-milliseconds-p-99-and-p-50').should('exist').within(DNSLatencies => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'DNS request rate per code and per namespace' panel
        cy.byLegacyTestID('panel-dns-request-rate-per-code-and-per-namespace').should('exist').within(DNSRequests => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'DNS latency per workload (milliseconds - p99 and p50)' panel
        // below 2 panels should appear with the flowcollector metric 'workload_dns_latency_seconds'
        cy.byLegacyTestID('panel-dns-latency-per-workload-milliseconds-p-99-and-p-50').should('exist').within(DNSLatencies => {
            cy.checkDashboards(appsInfra)
        })

        // verify 'DNS request rate per code and per workload' panel
        cy.byLegacyTestID('panel-dns-request-rate-per-code-and-per-workload').should('exist').within(DNSRequests => {
            cy.checkDashboards(appsInfra)
        })
    })

    after("Delete flowcollector and DNS pods", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI('oc delete -f ./fixtures/netobserv/DNS-pods.yaml')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogout()
    })
})
