import { Operator } from "../../views/netobserv"
import { netflowPage, genSelectors, colSelectors, querySumSelectors } from "../../views/netflow-page"

// if project name is changed here, it also needs to be changed 
// under fixture/flowcollector.ts and topology_view.spec.ts
const project = 'netobserv'

describe('(OCP-50532, OCP-50531, OCP-50530 NETOBSERV) Netflow Table view tests', function () {

    before('any test', function () {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.exec(`oc new-project ${project} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)

        // deploy loki
        cy.exec(`oc create -f ./fixtures/netobserv/loki.yaml -n ${project} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)

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
            let warningExists = false
            cy.get(querySumSelectors.queryStatsPanel).should('exist').then(qrySum => {
                if (Cypress.$(querySumSelectors.queryStatsPanel + ' svg.query-summary-warning').length > 0) {
                    warningExists = true
                }
            })

            cy.get(querySumSelectors.flowsCount).then(flowsCnt => {
                let nflows = 0
                if (warningExists) {
                    nflows = Number(flowsCnt.text().split('+ flows')[0])

                }
                else {
                    nflows = Number(flowsCnt.text().split(' ')[0])
                }
                expect(nflows).to.be.greaterThan(0)
            })

            cy.get(querySumSelectors.bytesCount).then(bytesCnt => {
                let nbytes = 0
                if (warningExists) {
                    nbytes = Number(bytesCnt.text().split('+ ')[0])
                }
                else {
                    nbytes = Number(bytesCnt.text().split(' ')[0])
                }
                expect(nbytes).to.be.greaterThan(0)
            })

            cy.get(querySumSelectors.packetsCount).then(pktsCnt => {
                let npkts = 0
                if (warningExists) {
                    npkts = Number(pktsCnt.text().split('+ ')[0])
                }
                else {
                    npkts = Number(pktsCnt.text().split(' ')[0])
                }
                expect(npkts).to.be.greaterThan(0)
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
            cy.get('td:nth-child(3) span.co-resource-item__resource-name').should('exist').each(row => {
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
            cy.reload()
            cy.get('#table-body > tr:nth-child(1) > td:nth-child(4) > div > div > span').should('not.have.text', 'loki (3100)')

            cy.get(':nth-child(1) > .pf-c-chip-group__label').click()
            cy.get('#filters  > .pf-c-toolbar__item > :nth-child(1)').should('not.have.class', '.disabled-value')

            cy.get('#table-body tr:nth-child(1) td:nth-child(4) span').should('have.text', 'loki (3100)')

            cy.byTestID('clear-all-filters-button').click()
            cy.get('div.custom-chip').should('not.exist')
        })

        it("should validate localstorage for plugin", function () {
            // clear all filters if present
            cy.get('body').then((body) => {
                if (body.find('[data-test="clear-all-filters-button"]').length > 0) {
                    cy.byTestID('clear-all-filters-button').click()
                }
            });

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

            //expand Source filter if its not expanded
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
                const settings = JSON.parse(localStorage.getItem('netobserv-plugin-settings'))
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
        if (this.catalogSource != "community-operators") {
            Operator.deleteCatalogSource(this.catalogSource)
        }
        cy.exec(`oc delete project ${project} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        cy.logout()

    })
})
