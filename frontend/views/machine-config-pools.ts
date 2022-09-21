
export const mcp = {
  listPage: {
    goToMCPPage: () => {
      cy.visit(
        "/k8s/cluster/machineconfiguration.openshift.io~v1~MachineConfigPool"
      );
    },

    checkUpdateStatus: (resourceName: string, status: string) => {
      cy.get(`[data-test-rows="resource-row"]`)
        .contains(resourceName)
        .parents('tr')
        .within(() => {
          cy.contains(status).should('be.visible')
        });
    }
  }
}
