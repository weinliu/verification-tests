import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"

const pagesToVisit = ["/k8s/ns/netobserv/core~v1~Pod",
    "/k8s/ns/netobserv/apps~v1~Deployment",
    "/k8s/ns/netobserv/daemonsets",
    "/k8s/ns/netobserv/apps~v1~ReplicaSet",
]

describe('(OCP-70972 Network_Observability) Netflow traffic pages on workloads', { tags: ['Network_Observability'] }, function () {

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

    it('(OCP-70972, memodi, Network_Observability), netflow traffic pages should appear on workloads', { tags: ["e2e", "admin", "@smoke"] }, function () {
        pagesToVisit.forEach((page) => {
            cy.visitNetflowTrafficTab(page)
        })
    })
    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })

})