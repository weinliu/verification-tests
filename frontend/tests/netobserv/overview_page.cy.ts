import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, genSelectors, querySumSelectors, overviewSelectors } from "../../views/netflow-page"

describe('(OCP-54839 NETOBSERV) Netflow Overview page tests', { tags: ['NETOBSERV'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
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

    describe("overview page features", function () {
        beforeEach('overview page test', function () {
            netflowPage.visit()
            netflowPage.waitForLokiQuery()
            cy.get('.overviewTabButton').should('exist')

            cy.checkPanel(overviewSelectors.defaultPanels)
            cy.checkPanelsNum(4);
        })

        it("(OCP-54839, aramesha) should validate overview page features", function () {

            cy.byTestID(genSelectors.timeDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.byTestID('1h').should('exist').click()
                })
            })

            cy.byTestID(genSelectors.refreshDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.byTestID('15s').should('exist').click()
                })
            })

            cy.byTestID(genSelectors.refreshBtn).should('exist').click()

            cy.showAdvancedOptions().then(views => {
                cy.contains('Display options').should('exist').click()

                cy.byTestID(overviewSelectors.typeDrop).then(btn => {
                    expect(btn).to.exist
                    cy.wrap(btn).click().then(drop => {
                        cy.byTestID('packets').should('exist').click()
                    })
                    cy.get(querySumSelectors.packetsCount).should('exist').then(packetsCnt => {
                        cy.checkQuerySummary(packetsCnt)
                        let metricType = String(packetsCnt.text().split(' ')[3])
                        expect(metricType).to.contain("packets")
                    })
                })

                cy.byTestID(overviewSelectors.scopeDrop).then(btn => {
                    expect(btn).to.exist
                    cy.wrap(btn).click().then(drop => {
                        cy.byTestID('resource').should('exist').click()
                    })
                })

                cy.byTestID(overviewSelectors.truncateDrop).then(btn => {
                    expect(btn).to.exist
                    cy.wrap(btn).click().then(drop => {
                        cy.byTestID('25').should('exist').click()
                    })
                })
            })
        })

        it("(OCP-54839, aramesha) should validate query summary panel", function () {
            cy.get(querySumSelectors.bytesCount).should('exist').then(bytesCnt => {
                cy.checkQuerySummary(bytesCnt)
            })

            cy.get(querySumSelectors.bpsCount).should('exist').then(bpsCnt => {
                cy.checkQuerySummary(bpsCnt)
            })
            cy.get('#query-summary-toggle').should('exist').click()
            cy.get('#summaryPanel').should('be.visible')

            cy.contains('Results').should('exist')
            cy.contains('Cardinality').should('exist')
            cy.contains('Configuration').should('exist')
            cy.contains('Sampling').should('exist')
        })

        it("(OCP-54839, aramesha) should validate panels", function () {
            //open panels modal
            cy.openPanelsModal();

            //check if all panels are listed 
            var panels: string[] = ['Top X average rates (donut)', 'Top X latest rates (donut)', 'Top X flow rates stacked (bars)', 'Total rate (line)', 'Top X flow rates stacked with total (bars)', 'Top X flow rates (lines)']
            cy.checkPopupItems(overviewSelectors.panelsModal, panels);

            //select all panels
            cy.get(overviewSelectors.panelsModal).contains('Select all').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanelsNum(6);

            //check if all panels are rendered
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.allPanels)

            //unselect all panels and check if save is disabled
            cy.openPanelsModal();
            cy.get(overviewSelectors.panelsModal).contains('Unselect all').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').should('be.disabled');

            //select 1 panel and check if its visible on console
            cy.selectPopupItems(overviewSelectors.panelsModal, ['Total rate (line)']);
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanel([overviewSelectors.allPanels[3]])
            cy.checkPanelsNum(1);

            //restrore default panels and check if visible on console
            cy.openPanelsModal();
            cy.get(overviewSelectors.panelsModal).contains('Restore default panels').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.defaultPanels)
            cy.checkPanelsNum();
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.logout()
    })
})