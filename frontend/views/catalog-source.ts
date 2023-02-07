export const catalogSources = {
    navToOperatorHubSources: () => {
        cy.visit('k8s/cluster/config.openshift.io~v1~OperatorHub/cluster/sources')
        cy.get('#yaml-create').should('exist')
    },
    enableQECatalogSource: (catalogName, catalogDisplayName) => {
        // TODO: apply ICSP and may be pull secrets needs to be added.
        catalogSources.navToOperatorHubSources();
        cy.get('#yaml-create').should('exist').click()
        cy.get("#catalog-source-name").type(catalogName)
        cy.get('#catalog-source-display-name').type(catalogDisplayName)
        cy.get('#catalog-source-image').type('quay.io/openshift-qe-optional-operators/ocp4-index:latest')
        cy.get('#save-changes').click()
        cy.byTestID(catalogName + '-status', { timeout: 60000 }).should('exist').should('have.text', "READY")
    },
    createCustomCatalog: (image: any, catalogSource: string, displayName: string) => {
        catalogSources.navToOperatorHubSources()
        if (catalogSource != "community-operators" && image == null) {
            throw new Error("Operator catalog image must be specified for catalogSource other than community-operator");
        }

        if (catalogSource != "community-operators") {
            cy.byTestID('item-create').should('exist').click()
            cy.byTestID('catalog-source-name').type(catalogSource)
            cy.get('#catalog-source-display-name').type(displayName)
            cy.get('#catalog-source-publisher').type('ocp-qe')
            cy.byTestID('catalog-source-image').type(image)
            cy.byTestID('save-changes').click()
        }

        cy.byTestID(catalogSource).should('exist')
        cy.byTestID(catalogSource + '-status', { timeout: 60000 }).should('have.text', "READY")
    }
}
