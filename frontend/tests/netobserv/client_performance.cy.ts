import { netflowPage, loadTimes, memoryUsage } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"

function getTopologyScopeURL(scope: string): string {
    return `**/topology?filters=&limit=50&recordType=flowLog&dedup=true&packetLoss=all&timeRange=300&rateInterval=30s&step=15s&type=bytes&aggregateBy=${scope}`
}

describe("NETOBSERV Client Performances", { tags: ['NETOBSERV'] }, function () {
    before("tests", function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        if (Cypress.browser.name != "chrome") {
            this.skip()
        }
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

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
        Operator.createFlowcollector(project)
    })

    beforeEach("test", function () {
        cy.clearLocalStorage()
        cy.clearCookies()
    })

    it("should measure overview page load times", function () {
        cy.visit("/netflow-traffic")
        netflowPage.clearAllFilters()
        const start = performance.now()
        cy.intercept('GET', getTopologyScopeURL("namespace"), {
            fixture: 'netobserv/overview_perf_ns.json'
        })
        cy.intercept('GET', getTopologyScopeURL("app"), {
            fixture: 'netobserv/overview_perf_app.json'
        })

        cy.get("#top_avg_donut").should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let memoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Overview page load took ${pageload} ms.`)
                cy.log(`Overview page memory consumption ${memoryUsage} MB`)
                cy.checkPerformance("overview", pageload, memoryUsage)
            })
        })
    })

    it("should measure table page load times", function () {
        cy.visit("/netflow-traffic")
        cy.get('#tabs-container li:nth-child(2)').click()
        netflowPage.clearAllFilters()
        const start = performance.now()
        const url = '**/loki/flows?filters=&limit=50&recordType=flowLog&dedup=true&packetLoss=all&timeRange=300&rateInterval=30s&step=15s&type=count'
        cy.intercept('GET', url, {
            fixture: 'netobserv/netflow_table_perf.json'
        })
        cy.byTestID("table-composable").should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let memoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Table view page load took ${pageload} ms.`)
                cy.log(`Table view memory consumption ${memoryUsage} MB`)
                cy.checkPerformance("table", pageload, memoryUsage)
            })
        })
    })

    it("should measure topology page load times", function () {
        cy.visit("/netflow-traffic")
        cy.get('#tabs-container li:nth-child(3)').click()
        netflowPage.clearAllFilters()
        const start = performance.now()
        cy.intercept('GET', getTopologyScopeURL("namespace"), {
            fixture: 'netobserv/topology_perf.json'
        })
        cy.get('[data-surface="true"]').should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let memoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Topology view page load took ${pageload} ms.`)
                cy.log(`Topology view memory consumption ${memoryUsage} MB`)
                cy.checkPerformance("topology", pageload, memoryUsage)
            })
        })
    })
    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("suite", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()

    })
})
