import { Operator, project } from "../../views/netobserv"
import { netflowPage } from "../../views/netflow-page"
var patch = [{
    "op": "$op",
    "path": "/spec/consolePlugin/quickFilters",
    "value": [
        {
            "default": true,
            "filter": {
                "dst_namespace": "test-client",
                "src_namespace": "test-server"
            },
            "name": "Test NS"
        }
    ]
}]

describe('(OCP-56222 Network_Observability) Quick Filters test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');
        // create test server and client pods
        cy.adminCLI('oc create -f ./fixtures/netobserv/test-server-client.yaml')

        Operator.install()
        Operator.createFlowcollector(project)

    })
    beforeEach('any netflow table test', function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')
    })


    it("(OCP-56222, memodi, Network_Observability) should verify quick filters add", function () {
        const addQuickFilterPatch = JSON.stringify(patch).replace('$op', 'add')
        cy.adminCLI(`oc patch flowcollector/cluster --type json -p \'${addQuickFilterPatch}\'`)
        // wait 10 seconds for plugin pod to get restarted
        cy.wait(10000)
        cy.reload()
        cy.contains("Quick filters").should('exist').click()
        cy.get('#quick-filters-dropdown').should('exist').contains("Test NS").children('[type="checkbox"]').check()

        // verify source and destination NS are test-server and test-client respectively
        cy.get('[data-test-td-column-id=SrcK8S_Namespace]').each((td) => {
            expect(td).attr("data-test-td-value").to.contain('test-server')
        })
        cy.get('[data-test-td-column-id=DstK8S_Namespace]').each((td) => {
            expect(td).attr("data-test-td-value").to.contain('test-client')
        })

        cy.get('[role="listbox"]').contains("Test NS").children('[type="checkbox"]').uncheck()
        cy.get('#filters > div').should('not.have.class', 'custom-chip-group')
    })

    it("(OCP-56222, memodi, Network_Observability) should verify quick filters remove", function () {
        const addQuickFilterPatch = JSON.stringify(patch).replace('$op', 'remove')
        cy.adminCLI(`oc patch flowcollector/cluster --type json -p \'${addQuickFilterPatch}\'`)

        // wait 10 seconds for plugin pod to get restarted
        cy.wait(10000)
        cy.reload()
        cy.contains("Quick filters").should('exist').click()
        cy.get('#quick-filters-dropdown label').should('exist').each((ele, index, $list) => {
            cy.wrap(ele).should('not.contain', "Test NS")
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI('oc delete -f ./fixtures/netobserv/test-server-client.yaml')
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
