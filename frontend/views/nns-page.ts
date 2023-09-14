export const nnsPage = {
    goToNNS: () => {
        cy.contains('Networking').should('be.visible');
        cy.clickNavLink(['Networking', 'NodeNetworkState']);
    },
};
