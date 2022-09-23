export const crds = {
  navToCRDsPage: () => cy.visit('/k8s/cluster/customresourcedefinitions'),
  checkNoMachineResources: () => {
    crds.navToCRDsPage();
    cy.get('input[data-test="name-filter-input"]').type("machine");
    cy.get('td').should('not.contain', 'Machine');
  }
}
