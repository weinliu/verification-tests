import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage } from "../../views/netflow-page"

describe('(OCP-74049, OCP-73875 Network_Observability) Prometheus datasource only', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project, "LokiDisabled")
    })

    it('(OCP-74049, aramesha, Network_Observability), Verify Prom dataSource in Administrator view as cluster-admin user', function () {
        netflowPage.visit()
        
        cy.checkNetflowTraffic("Disabled")

        // verify only prom and auto dataSource is enabled in query options
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
        cy.get('#query-options-dropdown').click();
        cy.get('#dataSource-loki').should('be.disabled')
        cy.get('#dataSource-prom').should('not.be.disabled')
        cy.get('#dataSource-auto').should('not.be.disabled')
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();

        // verify resource scope is not observed with prom dataSource
        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('scope-dropdown').click()
            cy.byTestID('resource').should('not.exist')
        })
    })

    it('(OCP-73876, aramesha, Network_Observability), Verify prom dataSource in Administrator view as non-cluster-admin user', function () {
        // Add user to netobserv-metrics-reader role to view metrics
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-monitoring-view ${Cypress.env('LOGIN_USERNAME')}`)

        // Add edit role to user for netobserv NS
        cy.adminCLI(`oc adm policy add-role-to-user edit ${Cypress.env('LOGIN_USERNAME')} -n ${project}`)

        // Remove user from cluster-admin role
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)

        cy.wait(5000)

        netflowPage.visit()

        cy.checkNetflowTraffic("Disabled")
    }) 
    after("after all tests are done", function () {
        cy.adminCLI(`oc delete clusterRoleBinding cluster-monitoring-view`)
        cy.adminCLI(`oc adm policy remove-role-from-user edit ${Cypress.env('LOGIN_USERNAME')} -n ${project}`)
        
        // Delete flowcollector
        cy.adminCLI(`oc delete flowcollector cluster`)
    })
})
