import { netflowPage, genSelectors, topologySelectors } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"

const metricFunction = [
    "Latest rate",
    "Average rate",
    "Min rate",
    "Max rate",
    "Total"
]
const metricType = [
    "Bytes",
    "Packets"
]

describe("(OCP-53591 Network_Observability) Netflow Topology view features", { tags: ['Network_Observability'] }, function () {
    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

        Operator.install()
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

        // advance options menu remains visible throughout the test
    })

    it("(OCP-53591, memodi, Network_Observability) should verify topology page features", { tags: ['@smoke'] }, function () {
        cy.byTestID('search-topology-element-input').should('exist')
        cy.contains('Display options').should('exist').click()

        cy.byTestID(topologySelectors.metricsDrop).should('exist').click()
        cy.get(topologySelectors.metricsList).should('have.length', 5).each((item, index) => {
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

    it("(OCP-53591, memodi, Network_Observability) should verify local storage", function () {
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
            expect(topologySettings['metricType']).to.be.equal('Bytes')
            expect(topologySettings['nodeBadges']).to.be.false
            expect(settings['netflow-traffic-metric-scope']).to.be.equal('namespace')
            expect(topologySettings['truncateLength']).to.be.equal(25)
        })
    })

    it("(OCP-53591, memodi, Network_Observability) should verify side panel", function () {
        cy.get('g[data-kind="node"] > g').eq(1).parent().should('exist').click()
        cy.get('#elementPanel').should('be.visible')

        // details tab
        cy.get('#drawer-tabs > ul > li:nth-child(1)').should('exist')
        // metrics tab
        cy.get('#drawer-tabs > ul > li:nth-child(2)').should('exist').click()
        cy.get('div.pf-c-chart').should('exist')
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
