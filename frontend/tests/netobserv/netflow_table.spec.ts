import { operatorHubPage } from "../../views/operator-hub-page"
import { NetObserv, Operator } from "../../views/netobserv"
import { OCCreds, OCCli } from "../../views/cluster-cliops"
import { netflowPage, genSelectors, colSelectors, querySumSelectors } from "../../views/netflow-page"

const project = 'network-observability'

describe('(OCP-50532, OCP-50531, NETOBSERV) Console Network Policies form tests', function () {

    before('any test', function () {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)

        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        let creds: OCCreds = { idp: Cypress.env('LOGIN_IDP'), user: Cypress.env('LOGIN_USERNAME'), password: Cypress.env('LOGIN_PASSWORD') }

        this.cli = new OCCli(creds)
        this.cli.create_project(project)
        let netobserv = new NetObserv(this.cli)
        netobserv.deploy_loki()
    })

    describe('install NetObserv Operator and flowcollector', function () {
        it("should deploy NOO and create flowcollector", function () {
            operatorHubPage.goTo()
            operatorHubPage.isLoaded()
            operatorHubPage.install("NetObserv Operator")
            cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')

            cy.contains('Flow Collector').invoke('attr', 'href').then(href => {
                cy.visit(href)
            })

            cy.byTestID('item-create').should('exist').click()
            cy.get('#root_spec_ipfix_accordion-toggle').click()
            cy.get('#root_spec_ipfix_sampling').clear().type('2')
            cy.byTestID('create-dynamic-form').click()


            cy.byTestID('toast-action', { timeout: 60000 }).should('exist')
            cy.reload(true)

            netflowPage.visit()
        })
    })

    describe("netflow table page features", function () {
        before('any netflow table test', function () {
            netflowPage.visit()
        })

        it("should validate netflow table features", function () {
            cy.byTestID(genSelectors.timeDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.get('[data-test="1h"]').should('exist').click()
                })
            })


            cy.byTestID(genSelectors.refreshDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.get('[data-test="15s"]').should('exist').click()
                })
            })

            cy.byTestID(genSelectors.refreshBtn).should('exist').click()

            cy.byTestID(genSelectors.moreOpts).should('exist').click().then(moreOpts => {
                cy.get(genSelectors.compact).click()
                cy.wrap(moreOpts).click()
                cy.get(genSelectors.large).click()
                cy.wrap(moreOpts).click()
                cy.get(genSelectors.expand).click()
                cy.get('#page-sidebar').then(sidenav => {
                    cy.byLegacyTestID('perspective-switcher-menu').should('not.be.visible')

                })
                cy.wrap(moreOpts).click()
                cy.get(genSelectors.expand).click()
                cy.byLegacyTestID('perspective-switcher-menu').should('exist')
            })
        })

        it("should validate query summary panel", function () {
            cy.get(querySumSelectors.queryStatsPanel).should('exist')
            cy.get(querySumSelectors.flowsCount).then(flowsCnt => {
                let nflows = flowsCnt.text().split(' ')[0]
                expect(Number(nflows)).to.be.greaterThan(0)
            })

            cy.get(querySumSelectors.bytesCount).then(bytesCnt => {
                let nbytes = bytesCnt.text().split(' ')[0]
                expect(Number(nbytes)).to.be.greaterThan(0)
            })

            cy.get(querySumSelectors.packetsCount).then(pktsCnt => {
                let npkts = pktsCnt.text().split(' ')[0]
                expect(Number(npkts)).to.be.greaterThan(0)
            })
        })

        it("should validate columns", function () {
            cy.byTestID(colSelectors.mColumns).click().then(col => {
                cy.get(colSelectors.columnsModal).should('be.visible')
            })
            // group columns
            cy.get('#K8S_OwnerObject').check()
            cy.get('#Mac').check()
            cy.get('#AddrPort').check()
            cy.get('#Proto').check()
            cy.get('#FlowDirection').check()

            // source columns 
            cy.get('#SrcK8S_HostIP').check()
            cy.get('#SrcK8S_Namespace').uncheck()

            // dest columns
            cy.get('#DstK8S_HostIP').check()

            cy.byTestID(colSelectors.save).click()

            cy.byTestID('table-composable').within(() => {
                cy.get(colSelectors.dstNodeIP).should('exist')
                cy.get(colSelectors.gK8sOwner).should('exist')
                cy.get(colSelectors.Mac).should('exist')
                cy.get(colSelectors.gIPPort).should('exist')
                cy.get(colSelectors.Protocol).should('exist')

                cy.get(colSelectors.srcNodeIP).should('exist')
                cy.get(colSelectors.srcNS).should('not.exist')

                cy.get(colSelectors.direction).should('exist')
            })

            // restore defaults
            cy.byTestID(colSelectors.mColumns).click().byTestID(colSelectors.resetDefault).click().byTestID(colSelectors.save).click()

            cy.byTestID('table-composable').within(() => {
                cy.get(colSelectors.srcNS).should('exist')
                cy.get(colSelectors.Mac).should('not.exist')
            })
        })

        it('should validate query options', function () {
            cy.get('.pf-c-select__toggle').click()
            cy.byTestID('query-options-dropdown').should('be.visible').within(() => {
                cy.byTestID('limit-100').click()
                cy.byTestID('limit-500').click()
                cy.byTestID('limit-1000').click()
            })
        })

        it("should validate filters", function () {
            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')

            // Verify Source namespace filter
            cy.byTestID('group-1-toggle').click().byTestID('src_namespace').click()
            cy.byTestID('autocomplete-search').type(project + '{enter}')
            cy.get('#filters div.custom-chip > p').should('have.text', `"${project}"`)

            // Verify NS column for all rows
            cy.get('td:nth-child(3) a').each(row => {
                cy.wrap(row).should('have.text', project)
            })

            // Verify filters can be cleared
            cy.byTestID('clear-all-filters-button').click()
            cy.get('div.custom-chip').should('not.exist')

            // Verify src port filter and port Naming
            cy.byTestID("column-filter-toggle").click()
            cy.byTestID('src_port').click()
            cy.byTestID('autocomplete-search').type('3100{enter}')
            cy.get('#filters div.custom-chip > p').should('have.text', 'loki')

            // Verify first row has correct text
            cy.get('#table-body tr:nth-child(1) td:nth-child(4) span').should('have.text', 'loki (3100)')

            // check enabled or disabling filter
            cy.get(':nth-child(1) > .pf-c-chip-group__label').click()
            cy.get('#filters  > .pf-c-toolbar__item > :nth-child(1)').should('have.class', 'disabled-group')

            // sort by port
            cy.get('[data-test=th-SrcPort] > .pf-c-table__button').click()
            cy.get('#table-body tr:nth-child(1) td:nth-child(4) span').should('not.have.text', 'loki (3100)')

            cy.get(':nth-child(1) > .pf-c-chip-group__label').click()
            cy.get('#filters  > .pf-c-toolbar__item > :nth-child(1)').should('not.have.class', '.disabled-value')

            cy.get('#table-body tr:nth-child(1) td:nth-child(4) span').should('have.text', 'loki (3100)')

            cy.byTestID('clear-all-filters-button').click()
            cy.get('div.custom-chip').should('not.exist')
        })

        it("should validate localstorage for plugin", function () {
            cy.byTestID(genSelectors.refreshDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.get('[data-test="15s"]').should('exist').click()
                })
            })

            // select compact column size
            cy.byTestID(genSelectors.moreOpts).click().then(() => {
                cy.get(genSelectors.compact).click()
            })

            cy.byTestID(colSelectors.mColumns).click().then(col => {
                cy.get(colSelectors.columnsModal).should('be.visible')
                cy.get('#StartTime').check()
                cy.byTestID(colSelectors.save).click()
            })

            cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')

            cy.get('#group-1-toggle').then($srcfilter => {
                if ($srcfilter.hasClass("pf-m-expanded")) {
                    cy.byTestID('src_port').click()
                    cy.byTestID('autocomplete-search').type('3100{enter}')
                    cy.get(':nth-child(1) > .pf-c-chip-group__label').click()
                }
                else {
                    cy.wrap($srcfilter).click()
                    cy.byTestID('src_port').click()
                    cy.byTestID('autocomplete-search').type('3100{enter}')
                    cy.get(':nth-child(1) > .pf-c-chip-group__label').click()
                }
            })


            cy.visit('/monitoring/alerts')
            netflowPage.visit()

            cy.get('#pageHeader').should('exist').then(() => {
                const settings = JSON.parse(localStorage.getItem('network-observability-plugin-settings'))
                expect(settings['netflow-traffic-refresh']).to.be.equal(15000)
                expect(settings['netflow-traffic-size-size']).to.be.equal('s')
                expect(settings['netflow-traffic-columns']).to.include('StartTime')
                expect(settings['netflow-traffic-disabled-filters']['src_port']).to.be.equal('3100')
            })
        })
    })

    after("delete flowcollector and NetObs Operator", function () {
        // uninstall operator and all resources 
        Operator.deleteFlowCollector()
        Operator.uninstall()
        this.cli.delete_project(project)
    })
})
