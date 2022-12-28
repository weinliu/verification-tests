import { netflowPage, genSelectors, topologySelectors, topologyPage } from "../../views/netflow-page"
import { Operator } from "../../views/netobserv"

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
    return `**/topology?filters=&limit=100&reporter=destination&layer=application&timeRange=300&type=bytes&scope=${scope}&rateInterval=30s&step=15s`
}

function getTopologyResourceScopeGroupURL(groups: string): string {
    return `**/topology?filters=&limit=100&reporter=destination&layer=application&timeRange=300&type=bytes&scope=resource&groups=${groups}&rateInterval=30s&step=15s`
}

describe("(OCP-53591 NETOBSERV) Netflow Topology view features", function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.adminCLI(`oc new-project ${project}`)

        // deploy loki
        cy.adminCLI(`oc create -f ./fixtures/netobserv/loki.yaml -n ${project}`)

        // sepcify --env noo_catalog_src=community-operators to run tests for community operator NOO release
        let catalogImg, catalogDisplayName
        if (Cypress.env('noo_catalog_src') == "community-operators") {
            catalogImg = null
            this.catalogSource = Cypress.env('noo_catalog_src')
            catalogDisplayName = "Community"
        }
        else {
            catalogImg = 'quay.io/netobserv/network-observability-operator-catalog:vmain'
            this.catalogSource = "netobserv-test"
            catalogDisplayName = "NetObserv QE"
        }
        Operator.createCustomCatalog(catalogImg, this.catalogSource)
        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project)
    })

    beforeEach("run before each test", function () {
        netflowPage.visit()
        cy.get('#pf-tab-topology-netflow-traffic-tabs').should('exist').click()
        cy.get('.pf-c-select__toggle').click()
        cy.byTestID('query-options-dropdown').should('be.visible').click().then(() => {
            cy.get('#layer-application').click()

        })
        cy.get('.pf-c-select__toggle').click()
        cy.get('#options').click()
        cy.byTestID('layout-dropdown').click()
        // set one display to test with
        cy.byTestID('Grid').click()

        cy.get(topologySelectors.optsClose).click()
        cy.byTestID(topologySelectors.metricsDrop).should('exist').click().get('#sum').click()
    })

    it("should verify topology page features", function () {
        cy.byTestID('topology-search-container').should('exist')
        cy.get('#options').should('exist')
        cy.byTestID(topologySelectors.metricsDrop).should('exist').click()
        cy.get(topologySelectors.metricsList).should('have.length', 4).each((item, index) => {
            cy.wrap(item).should('contain.text', metricFunction[index])
        })
        cy.byTestID('metricType').should('exist').click()
        cy.get('#metricType > ul > li').should('have.length', 2).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })
        cy.byTestID(genSelectors.timeDrop).should('exist')
        cy.byTestID(genSelectors.refreshDrop).should('exist')
        cy.get('#zoom-in').should('exist')
        cy.get('#zoom-out').should('exist')
        cy.get('#reset-view').should('exist')
        // available after https://issues.redhat.com/browse/NETOBSERV-591
        // cy.get('#query-summary').should('exist')
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
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 2)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 3)

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
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 7)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 7)
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
        cy.get('#drawer ' + topologySelectors.edge).should('have.length', 17)
        cy.get('#drawer ' + topologySelectors.node).should('have.length', 12)
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
        cy.get(topologySelectors.nGroups).should('have.length', 16)
    })

    it("should verify group Nodes+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('hosts%2Bowners'), { fixture: 'netobserv/topology_ghostsOwners.json' })
        topologyPage.selectScopeGroup("resource", "hosts+owners")
        // verify number of groups
        cy.get(topologySelectors.nGroups).should('have.length', 18)
    })

    it("should verify group NS", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces'), { fixture: 'netobserv/topology_gNS.json' })
        topologyPage.selectScopeGroup("resource", "namespaces")
        cy.get(topologySelectors.nGroups).should('have.length', 3)
    })

    it("should verify group NS+Owners", function () {
        cy.intercept('GET', getTopologyResourceScopeGroupURL('namespaces%2Bowners'), { fixture: 'netobserv/topology_gNSOwners.json' })
        topologyPage.selectScopeGroup("resource", "namespaces+owners")
        cy.get(topologySelectors.nGroups).should('have.length', 9)
    })

    it("should verify local storage", function () {
        // modify some options
        cy.get('#options').should('exist').click()
        cy.byTestID('truncate-dropdown').click().byTestID("25")
        cy.get(topologySelectors.badgeToggle).click()
        cy.get(topologySelectors.optsClose).click()
        cy.byTestID('scope-dropdown').should('exist').click().byTestID('namespace').should('exist').click()

        cy.visit('/monitoring/alerts')
        cy.visit('/netflow-traffic')
        cy.get('#drawer').should('exist')

        cy.get('#pageHeader').should('exist').then(() => {
            const settings = JSON.parse(localStorage.getItem('netobserv-plugin-settings'))
            const topologySettings = settings['netflow-traffic-topology-options']

            expect(settings['netflow-traffic-view-id']).to.be.equal('topology')
            expect(topologySettings['rangeInSeconds']).to.be.equal(300)
            expect(topologySettings['edgeTags']).to.be.true
            expect(topologySettings['edges']).to.be.true
            expect(topologySettings['groupTypes']).to.be.equal('none')
            expect(topologySettings['layout']).to.be.equal('Grid')
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
        cy.get('#resourceInfos').should('exist').within(() => {
            cy.get('#resourcelink').should('exist')
        })
        cy.get('#metrics').should('exist')
        cy.get('div.pf-c-chart').should('exist')
    })

    describe("groups, edges, labels, badges", function () {
        beforeEach("select owners as scope", function () {
            topologyPage.selectScopeGroup("resource", "owners")
            cy.get('#options').click()
        })

        it("should verify group owners", function () {
            cy.intercept(getTopologyResourceScopeGroupURL('owners'), { fixture: 'netobserv/topology_gOwners.json' })
            cy.get(topologySelectors.nGroups).should('have.length', 6)
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

            cy.get(topologySelectors.edgeToggle).click()
            // verify labels are also hidden
            cy.get('#edges-tag-switch').should('have.attr', 'aria-labelledby', 'edges-tag-switch-off')
            cy.get(topologySelectors.defaultLayer).each((node, index) => {
                cy.wrap(node).should('not.have.descendants', '' + topologySelectors.edge)
            })
            cy.get(topologySelectors.edgeToggle).click()
            cy.get('#edges-tag-switch').should('have.attr', 'aria-labelledby', 'edges-tag-switch-on')
            cy.get(topologySelectors.defaultLayer).each((node, index) => {
                cy.wrap(node).should('have.descendants', '' + topologySelectors.edge)
            })
        })

        it("should verify edges labels can be displayed/hidden", function () {
            cy.get(topologySelectors.labelToggle).click()
            cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
                cy.wrap(node).should('not.have.descendants', 'g.pf-topology__edge__tag')
            })
            cy.get(topologySelectors.labelToggle).click()
            cy.get(topologySelectors.defaultLayer + ' > ' + topologySelectors.edge).each((node, index) => {
                cy.wrap(node).should('have.descendants', 'g.pf-topology__edge__tag')
            })
        })

        it("should verify badges display/hidden", function () {
            cy.get(topologySelectors.badgeToggle).click()
            cy.get('g.pf-topology__node__label').each((node, index) => {
                cy.wrap(node).should('not.have.descendants', 'g.pf-topology__node__label__badge')
            })
            cy.get(topologySelectors.badgeToggle).click()
            cy.get('g.pf-topology__node__label').each((node, index) => {
                cy.wrap(node).should('have.descendants', 'g.pf-topology__node__label__badge')
            })
        })

        afterEach("after each tests", function () {
            cy.get(topologySelectors.optsClose).click()
        })
    })

    after("after all tests are done", function () {
        Operator.deleteFlowCollector()
        Operator.uninstall()
        if (this.catalogSource != "community-operators") {
            Operator.deleteCatalogSource(this.catalogSource)
        }
        cy.adminCLI(`oc delete project ${project}`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()
    })
})
