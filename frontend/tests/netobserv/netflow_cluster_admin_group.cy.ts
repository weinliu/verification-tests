import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage } from "../../views/netflow-page"

describe('(OCP-67617 Network_Observability) User in group with cluster-admin role', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        // create new group, add user to that group and give that group cluster-admin role
        cy.adminCLI(`oc adm groups new netobservadmins`)
        cy.adminCLI(`oc adm groups add-users netobservadmins ${Cypress.env('LOGIN_USERNAME')}`)
        cy.adminCLI(`oc adm policy add-cluster-role-to-group cluster-admin netobservadmins`)

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

    describe("User in group with cluster-admin role test", function () {
        beforeEach('any user in group with cluster-admin role test', function () {
            netflowPage.visit()            
        })

        it("(OCP-67617, aramesha, Network_Observability) should verify user in group with cluster-admin role is able to access flows", function () {
            // validate netflow traffic page
            cy.checkNetflowTraffic()
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("all tests", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-group cluster-admin netobservadmins`)
        cy.adminCLI(`oc adm groups remove-users netobservadmins ${Cypress.env('LOGIN_USERNAME')}`)
        cy.adminCLI(`oc delete groups netobservadmins`)
    })
})
