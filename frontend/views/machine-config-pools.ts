
export const mcp = {
  listPage: {
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
