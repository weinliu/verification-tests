import { netflowPage, genSelectors, topologySelectors, topologyPage } from "../../views/netflow-page"
import { Operator } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
// if project name is changed here, it also needs to be changed 
// under fixture/flowcollector.ts and netflow_table.spec.ts
const project = 'netobserv'
const metricFunction = [
    "Latest rate",
    "Average rate",
    "Max rate",
    "Total"
]
const metricType = [
    "Bytes",
    "Packets"
]

function getTopologyScopeURL(scope: string): string {
    return `**/topology?filters=*&limit=50&recordType=flowLog&reporter=destination&timeRange=300&rateInterval=30s&step=15s&type=bytes&scope=${scope}`
}

function getTopologyResourceScopeGroupURL(groups: string): string {
    return `**/topology?filters=*&limit=50&recordType=flowLog&reporter=destination&timeRange=300&rateInterval=30s&step=15s&type=bytes&scope=resource&groups=${groups}`
}
// NETOBSERV-784 bug can fail some cases where some topoloigy tests may crash console.

describe("(OCP-53591 NETOBSERV) Netflow Topology view features", { tags: ['NETOBSERV'] }, function () {
    // bug NETOBSERV-779 could fail some tests
    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.adminCLI(`oc new-project ${project}`)

        // deploy loki
        cy.adminCLI(`oc create -f ./fixtures/netobserv/loki.yaml -n ${project}`)
        cy.switchPerspective('Administrator');

        // sepcify --env noo_catalog_src=upstream to run tests 
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
            catalogSources.enableQECatalogSource()
        }
        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project)
    })

    beforeEach("run before each test", function () {
        cy.clearLocalStorage()
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
        cy.byTestID(topologySelectors.metricsDrop).should('exist').click().get('#sum').click()
        cy.contains('Display options').should('exist').click()

        // Advance options menu remains visible throughout the test
    })

    it.only("should verify topology page features", function () {
        cy.byTestID('search-topology-element-input').should('exist')
        cy.contains('Display options').should('exist').click()

        cy.byTestID(topologySelectors.metricsDrop).should('exist').click()
        cy.get(topologySelectors.metricsList).should('have.length', 4).each((item, index) => {
            cy.wrap(item).should('contain.text', metricFunction[index])
        })
        cy.byTestID('metricType').should('exist').click()
        cy.get('#metricType > ul > li').should('have.length', 2).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })
        cy.contains('Display options').should('exist').click()

        cy.byTestID(genSelectors.timeDrop).should('exist')
        cy.byTestID(genSelectors.refreshDrop).should('exist')
        cy.get('#zoom-in').should('exist')
        cy.get('#zoom-out').should('exist')
        cy.get('#reset-view').should('exist')
        cy.get('#query-summary').should('exist')
    })

    it("should verify namespace scope", function () {
        const scope = "namespace"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/topology_namespace.json'
        }).as('matchedUrl')

        // selecting something different first  
        // to re-trigger API request on namespace selection
        topologyPage.selectScopeGroup("owner", null)
        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 4)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 5)

    })

    it("should verify owner scope", function () {
        const scope = "owner"
        cy.intercept('GET', getTopologyScopeURL(scope), {
            fixture: 'netobserv/topology_owner.json'
        }).as('matchedUrl')
        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 19)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 16)
    })

    it("should verify resource scope", function () {
        const scope = 'resource'
        cy.intercept('GET', getTopologyScopeURL(scope), { fixture: 'netobserv/topology_resource.json' }).as('matchedUrl')
        topologyPage.selectScopeGroup(scope, null)
        cy.wait('@matchedUrl').then(({ response }) => {
            expect(response.statusCode).to.eq(200)
        })
        topologyPage.isViewRendered()
        // verify number of edges and nodes.
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 46)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 28)
    })

    it("should verify group Nodes", function () {
        const groups = 'hosts'
        cy.intercept('GET', getTopologyResourceScopeGroupURL(groups), {
            fixture: 'netobserv/topology_ghosts.json'
        })
        topologyPage.selectScopeGroup("resource", groups)
        topologyPage.isViewRendered()
        // verify number of groups, to be equal to number of cluster nodes
        cy.get(topologySelectors.nGroups).should('have.length', 6)
    })

    it("should verify group Nodes+NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bnamespaces'), { fixture: 'netobserv/topology_ghostsNS.json' })
        topologyPage.selectScopeGroup("resource", "hosts+namespaces")
        topologyPage.isViewRendered()
        cy.get(topologySelectors.nGroups).should('have.length', 6)
    })

    it("should verify group Nodes+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bowners'), { fixture: 'netobserv/topology_ghostsOwners.json' })
        topologyPage.selectScopeGroup("resource", "hosts+owners")
        // verify number of groups
        cy.get(topologySelectors.nGroups).should('have.length', 20)
    })

    it("should verify group NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces'), { fixture: 'netobserv/topology_gNS.json' })
        topologyPage.selectScopeGroup("resource", "namespaces")
        cy.get(topologySelectors.nGroups).should('have.length', 4)
    })

    it("should verify group NS+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces%2Bowners'), { fixture: 'netobserv/topology_gNSOwners.json' })
        topologyPage.selectScopeGroup("resource", "namespaces+owners")
        cy.get(topologySelectors.nGroups).should('have.length', 20)
    })

    it("should verify local storage", function () {
        // modify some options
        cy.contains('Display options').should('exist').click()
        cy.byTestID('truncate-dropdown').click().byTestID("25")
        cy.get(topologySelectors.badgeToggle).uncheck()
        cy.byTestID('scope-dropdown').should('exist').click().byTestID('namespace').should('exist').click()
        cy.contains('Display options').should('exist').click()

        cy.visit('/monitoring/alerts')
        cy.visit('/netflow-traffic')
        cy.get('#drawer').should('exist')

        cy.get('#pageHeader').should('exist').then(() => {
            const settings = JSON.parse(localStorage.getItem('netobserv-plugin-settings'))
            const topologySettings = settings['netflow-traffic-topology-options']

            expect(settings['netflow-traffic-view-id']).to.be.equal('topology')
            expect(topologySettings['edgeTags']).to.be.true
            expect(topologySettings['edges']).to.be.true
            expect(topologySettings['groupTypes']).to.be.equal('none')
            // expect(topologySettings['layout']).to.be.equal('Grid')
            expect(topologySettings['metricFunction']).to.be.equal('last')
            expect(topologySettings['metricType']).to.be.equal('bytes')
            expect(topologySettings['nodeBadges']).to.be.false
            expect(settings['netflow-traffic-metric-scope']).to.be.equal('namespace')
            expect(topologySettings['truncateLength']).to.be.equal(25)
        })
    })

    it("should verify side panel", function () {
        cy.get('g[data-kind="node"] > g').eq(1).parent().should('exist').click()
        cy.get('#elementPanel').should('be.visible')

        // Details tab
        cy.get('#drawer-tabs > ul > li:nth-child(1)').should('exist')
        // Metrics tab
        cy.get('#drawer-tabs > ul > li:nth-child(2)').should('exist').click()
        cy.get('div.pf-c-chart').should('exist')
    })

    describe("groups, edges, labels, badges", function () {
        beforeEach("select owners as scope", function () {
            topologyPage.selectScopeGroup("resource", "owners")
            cy.contains('Display options').should('exist').click()
        })

        it("should verify group owners", function () {
            cy.intercept(getTopologyResourceScopeGroupURL('owners'), { fixture: 'netobserv/topology_gOwners.json' })
            cy.get(topologySelectors.nGroups).should('have.length', 13)
        })

        it("should verify group expand/collapse", function () {
            cy.get(topologySelectors.groupToggle).click()
            cy.get(topologySelectors.groupLayer + ' > ' + topologySelectors.group).each((node, index) => {
                cy.wrap(node).should('not.have.descendants', 'g.pf-topology__group')
            })
            cy.get(topologySelectors.groupToggle).click()
            cy.get(topologySelectors.groupLayer + ' > ' + topologySelectors.group).each((node, index) => {
                cy.wrap(node).should('have.descendants', 'g.pf-topology__group')
            })
        })

        it("should verify edges display/hidden", function () {
            cy.get(topologySelectors.edgeToggle).uncheck()

            // verify labels are also hidden
            cy.get('#edges-tag-switch').should('be.disabled')
            cy.get(topologySelectors.defaultLayer).each((node, index) => {
                cy.wrap(node).should('not.have.descendants', '' + topologySelectors.edge)
            })
            cy.get(topologySelectors.edgeToggle).check()
            cy.get('#edges-tag-switch').should('be.enabled')
            cy.get(topologySelectors.defaultLayer).each((node, index) => {
                cy.wrap(node).should('have.descendants', '' + topologySelectors.edge)
            })
        })

        it("should verify edges labels can be displayed/hidden", function () {
            cy.byTestID('autocomplete-search').should('exist').type(project + '{enter}')
            topologyPage.selectScopeGroup(null, "none")
            topologyPage.selectScopeGroup("namespace", null)
            cy.get('#reset-view').should('exist').click()

            cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
                cy.wrap(node).should('have.descendants', 'g.pf-topology__edge__tag')
            })
            cy.contains('Display options').should('exist').click()
            cy.get(topologySelectors.labelToggle).uncheck()
            // cy.contains('Display options').should('exist').click()

            cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
                cy.wrap(node).should('not.have.descendants', 'g.pf-topology__edge__tag')
            })
            cy.get(topologySelectors.labelToggle).check()
            cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
                cy.wrap(node).should('have.descendants', 'g.pf-topology__edge__tag')
            })
        })

        it("should verify badges display/hidden", function () {
            cy.byTestID('autocomplete-search').should('exist').type(project + '{enter}')
            topologyPage.selectScopeGroup(null, "none")
            topologyPage.selectScopeGroup("namespace", null)
            cy.get('#reset-view').should('exist').click()


            cy.contains('Display options').should('exist').click()
            cy.get(topologySelectors.badgeToggle).uncheck()


            cy.get('g.pf-topology__node__label').each((node, index) => {
                cy.wrap(node).should('not.have.descendants', 'g.pf-topology__node__label__badge')
            })

            // not checking the existence of the badge since there may be an 
            // "Unknown" node present with empty badge.
        })

        afterEach("after each tests", function () {
            cy.contains('Display options').should('exist').click()
        })
    })

    after("after all tests are done", function () {
        Operator.deleteFlowCollector()
        Operator.uninstall()
        cy.adminCLI('oc delete crd/flowcollectors.flows.netobserv.io')
        cy.adminCLI(`oc delete project ${project}`)
        cy.adminCLI('oc delete project openshift-netobserv-operator')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()
    })
})
