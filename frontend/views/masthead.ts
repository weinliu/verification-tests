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
  }
}