import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, genSelectors } from "../../views/netflow-page"

describe('(OCP-75874 Network_Observability) NetObserv developer view', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project)
    })

    it("(OCP-75874, aramesha, Network_Observability) should verify developer view - Loki DataSource", function () {
        netflowPage.visitDeveloper()
        cy.get('#tour-step-footer-secondary').should('exist').click()

        cy.byTestID(genSelectors.refreshDrop).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="15s"]').should('exist').click()
            })
        })

        // verify Netflow traffic tab
        cy.checkNetflowTraffic()
    })

    it("(OCP-75874, aramesha, Network_Observability) should verify developer view - Prom DataSource", function () {
        cy.switchPerspective('Administrator');
        // Delete and deploy flowcollector with Loki disabled
        Operator.deleteFlowCollector()
        Operator.createFlowcollector(project, "LokiDisabled")

        // Remove user from cluster-admin
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)

        // Add Role and RoleBindings for test user to view developer view
        cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/netobserv/prom-role-roleBinding.yaml -p NAMESPACE=${project} -p USERNAME=${Cypress.env('LOGIN_USERNAME')} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, { failOnNonZeroExit: false })
            .then(output => {
                expect(output.stderr).not.contain('Error');
            })
        // Add edit role to user for netobserv NS
        cy.adminCLI(`oc adm policy add-role-to-user edit ${Cypress.env('LOGIN_USERNAME')} -n ${project}`)

        netflowPage.visitDeveloper()

        cy.byTestID(genSelectors.refreshDrop).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="15s"]').should('exist').click()
            })
        })

        // verify Netflow traffic tab
        cy.checkNetflowTraffic("Disabled")
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-role-from-user edit ${Cypress.env('LOGIN_USERNAME')} -n ${project}`)
        cy.adminCLI(`oc delete roleBinding netobserv-prom-test -n ${project}`)
        cy.adminCLI(`oc delete role netobserv-prom -n ${project}`)
        
        // Delete flowcollector
        cy.adminCLI(`oc delete flowcollector cluster`)
    })
})
