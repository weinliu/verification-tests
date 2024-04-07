
export const mcp = {
  listPage: {
    checkUpdateStatus: (resourceName: string, status: any) => {
      cy.get(`[data-test-rows="resource-row"]`)
        .contains(resourceName)
        .parents('tr')
        .within(() => {
          cy.contains(status).should('be.visible')
        });
    }
  }
}
