import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, colSelectors, overviewSelectors, querySumSelectors } from "../../views/netflow-page"

describe('(OCP-68246 Network_Observability) FlowRTT test', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project, "FlowRTT")
    })

    beforeEach('any flowRTT test', function () {
        netflowPage.visit()
    })

    it("(OCP-68246, aramesha, Network_Observability) Verify flowRTT column values", function () {
        // go to table view
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')

        // verify flowRTT column is present by default
        cy.byTestID('table-composable').should('exist').within(() => {
            cy.get(colSelectors.flowRTT).should('exist')
        })

        // filter on Protocol TCP, all flows should have flowRTT value != n/a
        cy.byTestID("column-filter-toggle").click().get('.pf-c-dropdown__menu').should('be.visible')
        cy.byTestID('group-2-toggle').click().should('be.visible')
        cy.byTestID('protocol').click()
        cy.get('#autocomplete-search').type('TCP' + '{enter}')

        cy.get('[data-test-td-column-id=TimeFlowRttMs]').each((td) => {
            expect(td).attr("data-test-td-value").to.match(RegExp("^[0-9]*$"))
        })
    })

    it("(OCP-68246, aramesha, Network_Observability) Verify flowRTT panels", function () {
        // verify default flowRTT panels are visible
        cy.checkPanel(overviewSelectors.defaultFlowRTTPanels)
        cy.checkPanelsNum(5);

        // open panels modal and verify all relevant panels are listed
        cy.openPanelsModal();
        cy.checkPopupItems(overviewSelectors.panelsModal, overviewSelectors.manageFlowRTTPanelsList);

        // select all panels and verify they are rendered
        cy.get(overviewSelectors.panelsModal).contains('Select all').click();
        cy.get(overviewSelectors.panelsModal).contains('Save').click();
        netflowPage.waitForLokiQuery()
        cy.checkPanelsNum(9);
        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.allFlowRTTPanels)

        // restore default panels and verify they are visible
        cy.byTestID('view-options-button').click()
        cy.get(overviewSelectors.mPanels).click().byTestID(overviewSelectors.resetDefault).click().byTestID(overviewSelectors.save).click()
        netflowPage.waitForLokiQuery()
        cy.checkPanel(overviewSelectors.defaultFlowRTTPanels)
        cy.checkPanelsNum(5);

        // verify Query Summary stats for flowRTT
        cy.get(querySumSelectors.avgRTT).should('exist').then(avgRTT => {
            cy.checkQuerySummary(avgRTT)
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
