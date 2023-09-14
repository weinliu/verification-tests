export const nncpPage = {
    goToNNCP: () => {
        cy.contains('Networking').should('be.visible');
        cy.clickNavLink(['Networking', 'NodeNetworkConfigurationPolicy']);
    },
};
