export const searchPage = {
  navToSearchPage: () => cy.visit('/search/all-namespaces'),
  chooseResourceType: (resource_type) => {
    cy.get('input[placeholder="Resources"]').clear().type(`${resource_type}`);
    cy.get(`label[id$="~${resource_type}"]`).click();
  },
  checkNoMachineResources: () => {
    searchPage.navToSearchPage();
    cy.get('button[class*=c-select__toggle]').click();
    cy.get('[placeholder="Resources"]').type("machine");
    const machineResources = ['MMachine','MAMachineAutoscaler','MCMachineConfig','MCPMachineConfigPool','MHCMachineHealthCheck','MSMachineSet'];
    machineResources.forEach((machineResource) => {
      cy.get(`[data-filter-text=${machineResource}]`).should('not.exist');
    });
  },
  clearAllFilters: () => {
    cy.byButtonText('Clear all filters').click({force: true});
  },
  searchMethodValues: (method, value) => {
    method = method.toLocaleLowerCase();
    cy.get('button[id="search-filter-toggle"]').click();
    cy.get(`li[data-test="${method}-filter"] button[role="option"]`).click();
    cy.get('input[id="search-filter-input"]').clear().type(`${value}`);
  },
  searchBy: (text) => {
    cy.get('input[data-test-id="item-filter"]').clear().type(`${text}`)
  },
}
