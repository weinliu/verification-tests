import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage } from "../../views/netflow-page"

const [user1, user1Passwd] = Cypress.env('LOGIN_USERS').split(',')[0].split(':');
const [user2, user2Passwd] = Cypress.env('LOGIN_USERS').split(',')[1].split(':');
const [user3, user3Passwd] = Cypress.env('LOGIN_USERS').split(',')[2].split(':');

describe('(OCP-75874 Network_Observability) NetObserv developer view', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${user1}`)
        cy.login(Cypress.env('LOGIN_IDP'), user1, user1Passwd)
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
        netflowPage.visitDeveloper(project)
        cy.get('#tour-step-footer-secondary').should('exist').click()

        // verify Netflow traffic tab
        cy.checkNetflowTraffic()
    })

    it("(OCP-75874, aramesha, Network_Observability) should verify developer view - Prom DataSource", function () {
        cy.switchPerspective('Administrator');
        // Deploy flowcollector with Loki disabled
        Operator.createFlowcollector(project, "LokiDisabled")

        // Provide user2 and user3 with netobserv-metrics-reader role to view prom queries
        cy.adminCLI(`oc adm policy add-cluster-role-to-user netobserv-metrics-reader ${user2}`)
        cy.adminCLI(`oc adm policy add-cluster-role-to-user netobserv-metrics-reader ${user3}`)

        // Deploy client server manifests logged in as user1
        cy.cliLogin(`${user2}`, `${user2Passwd}`)
        cy.exec(`oc create -f ./fixtures/netobserv/testuser-server-client.yaml`)

        // Logout from console as user3 and login as user1
        cy.uiLogout()
        cy.login(Cypress.env('LOGIN_IDP'), user2, user2Passwd)
        cy.get('#tour-step-footer-secondary').should('exist').click()

        // Verify Netflow traffic tab Developer view as user1
        netflowPage.visitDeveloper("test-client")
        cy.checkNetflowTraffic("Disabled")

        // Add view role for test-server project to user3
        cy.exec(`oc adm policy add-role-to-user view ${user3} -n test-server`)

        // Logout from console as user3 and login as user3
        cy.uiLogout()
        cy.login(Cypress.env('LOGIN_IDP'), user3, user3Passwd)
        cy.get('#tour-step-footer-secondary').should('exist').click()

        // verify Netflow traffic tab as user3
        netflowPage.visitDeveloper("test-server")
        cy.checkNetflowTraffic("Disabled")
    })

    after("after all tests are done", function () {
        // Login as system:admin from CLI
        cy.exec(`oc login -u system:admin`)
        cy.adminCLI(`oc delete project test-client`)
        cy.adminCLI(`oc delete project test-server`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user netobserv-metrics-reader ${user2}`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user netobserv-metrics-reader ${user3}`)
        cy.adminCLI(`oc delete flowcollector cluster`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${user1}`)
    })
})
