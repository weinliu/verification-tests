export const catalogSources = {
    navToOperatorHubSources: () => {
        cy.visit('k8s/cluster/config.openshift.io~v1~OperatorHub/cluster/sources')
        cy.get('#yaml-create').should('exist')
    },
    getOCPVersion: () => {
        let cmd = `oc version -o json --kubeconfig=${Cypress.env('KUBECONFIG_PATH')} | grep openshiftVersion | awk -F'"' '{print $4}' | awk -F'.' '{print $1"."$2}' `
        cy.exec(cmd).then(result => {
            expect(result.stderr).to.be.empty
            cy.wrap(result.stdout).as('VERSION')
        })
    },
    enableQECatalogSource: (catalogName = "qe-app-registry", catalogDisplayName = "QE Catalog") => {
        // add QE catalog only if it does not exist.
        if (!catalogSources.checkCatalogSourcePresent(catalogName)) {
            cy.adminCLI(`oc create -f ./fixtures/icsp.yaml`)
            catalogSources.getOCPVersion()
            cy.get('@VERSION').then((VERSION) => {
                const INDEX_IMAGE = `quay.io/openshift-qe-optional-operators/aosqe-index:v${VERSION}`
                cy.log(`adding catalog source ${INDEX_IMAGE}`)
                catalogSources.navToOperatorHubSources();
                cy.get('#yaml-create').should('exist').click()
                cy.get("#catalog-source-name").type(catalogName)
                cy.get('#catalog-source-display-name').type(catalogDisplayName)
                cy.get('#catalog-source-image').type(INDEX_IMAGE)
                cy.get('#save-changes').click()
                cy.byTestID(catalogName + '-status', { timeout: 60000 }).should('exist').should('have.text', "READY")
            })
        }
    },
    createCustomCatalog: (image: any, catalogSource: string, displayName: string) => {
        catalogSources.navToOperatorHubSources()
        if (catalogSource != "community-operators" && image == null) {
            throw new Error("Operator catalog image must be specified for catalogSource other than community-operator");
        }
        // create custom catalog only if its not already present
        if (catalogSource != "community-operators" && !catalogSources.checkCatalogSourcePresent(catalogSource)) {
            cy.byTestID('item-create').should('exist').click()
            cy.byTestID('catalog-source-name').type(catalogSource)
            cy.get('#catalog-source-display-name').type(displayName)
            cy.get('#catalog-source-publisher').type('ocp-qe')
            cy.byTestID('catalog-source-image').type(image)
            cy.byTestID('save-changes').click()
        }

        cy.byTestID(catalogSource).should('exist')
        cy.byTestID(catalogSource + '-status', { timeout: 60000 }).should('have.text', "READY")
    },
    checkCatalogSourcePresent: (catalogName: string) => {
        return cy.exec(`oc get -n openshift-marketplace catalogsource ${catalogName} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
        .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return true;
        }
        else {
            return false;
        }
      })
    }
}
