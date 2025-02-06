import { Operator, project } from "../../views/netobserv"

const pagesToVisit = ["/k8s/ns/netobserv/core~v1~Pod",
    "/k8s/ns/netobserv/apps~v1~Deployment",
    "/k8s/ns/netobserv/daemonsets",
    "/k8s/ns/netobserv/apps~v1~ReplicaSet",
]

describe('(OCP-70972 Network_Observability) Netflow traffic pages on workloads', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project)
    })

    it('(OCP-70972, memodi, Network_Observability), netflow traffic pages should appear on workloads', { tags: ["@smoke"] }, function () {
        pagesToVisit.forEach((page) => {
            cy.clearLocalStorage()
            cy.visitNetflowTrafficTab(page)
        })
    })
    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })

})
