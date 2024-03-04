export const crds = {
  navToCRDsPage: () => cy.visit('/k8s/cluster/customresourcedefinitions'),
  checkNoMachineResources: () => {
    crds.navToCRDsPage();
    cy.get('input[data-test="name-filter-input"]').type("machine");
    const machineResources = ['Machine','MachineAutoscaler','MachineConfig','MachineConfigPool','MachineHealthCheck','MachineSet'];
    machineResources.forEach((machineResource) => {
      cy.get(`[data-test=${machineResource}]`).should('not.exist');
    });
  }
}
