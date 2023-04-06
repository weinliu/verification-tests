export const masthead = {
  clusterDropdownToggle: () => {
    cy.byLegacyTestID('cluster-dropdown-toggle').click();
  },
  filterClusters: (cluster_name: string) => {
    cy.get('.co-cluster-menu')
      .within(() => {
        cy.get('input[type="search"]')
          .clear()
          .type(cluster_name)
      });
  },
  changeToCluster: (cluster_name: string) => {
    cy.byButtonText(cluster_name).click();
  },
  clearFilters: () => {
    cy.byButtonText('Clear filter').click();
  },
  openHelpItem: (itemName: string) => {
    cy.get('button[aria-label="Help menu"]').click();
    cy.contains(`${itemName}`).click();
  },
  checkFeedbackModal: () => {
    cy.contains('Share feedback').should('exist');
    cy.contains(/Report a bug|Open a support case/).should('exist');
    cy.contains('Inform the direction of Red Hat').should('exist');
  },
  cancelFeedback: () => {
    cy.contains('Cancel').click();
    cy.get('[aria-label="Feedback modal"]').should('not.exist');
  }
}
