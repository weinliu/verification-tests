import { Operator, project } from "../../views/netobserv"
import { netflowPage } from "../../views/netflow-page"

const [user1, user1Passwd] = Cypress.env('LOGIN_USERS').split(',')[0].split(':');
const [user2, user2Passwd] = Cypress.env('LOGIN_USERS').split(',')[1].split(':');
const [user3, user3Passwd] = Cypress.env('LOGIN_USERS').split(',')[2].split(':');

describe('(OCP-75874 Network_Observability) NetObserv developer view', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${user1}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), user1, user1Passwd)

        Operator.install()
        Operator.createFlowcollector(project)
    })

    it("(OCP-75874, aramesha, Network_Observability) should verify developer view - Loki DataSource", function () {
        netflowPage.visitDeveloper(project)

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

        // Deploy client server manifests logged in as user2
        cy.cliLogin(`${user2}`, `${user2Passwd}`)
        cy.exec(`oc create -f ./fixtures/netobserv/testuser-server-client.yaml`)

        // Logout from console as user1 and login as user2
        cy.uiLogout().then(() => {
            cy.visit(Cypress.config('baseUrl'))
        })
        cy.uiLogin(Cypress.env('LOGIN_IDP'), user2, user2Passwd)

        // Verify Netflow traffic tab Developer view as user2
        netflowPage.visitDeveloper("test-client")
        cy.checkNetflowTraffic("Disabled")

        // Add view role for test-server project to user3
        cy.exec(`oc adm policy add-role-to-user view ${user3} -n test-server`)

        // Logout from console as user3 and login as user3
        cy.uiLogout().then(() => {
            cy.visit(Cypress.config('baseUrl'))
        })
        cy.uiLogin(Cypress.env('LOGIN_IDP'), user3, user3Passwd)

        // verify Netflow traffic tab as user3
        netflowPage.visitDeveloper("test-server")
        cy.checkNetflowTraffic("Disabled")
        // Login as system:admin from CLI
        cy.adminCLI(`oc login -u system:admin`)
        cy.adminCLI(`oc delete project test-client`)
        cy.adminCLI(`oc delete project test-server`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user netobserv-metrics-reader ${user2}`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user netobserv-metrics-reader ${user3}`)
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc delete flowcollector cluster`)
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${user1}`)
    })
})
