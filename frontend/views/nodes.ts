export const nodesPage = {
  goToNodesPage: () => {
    cy.visit('/k8s/cluster/core~v1~Node');
    cy.get('[data-test-id="resource-title"]',{timeout: 60000}).should('exist');
  },
  gotoDetail: (nodeName) => {
    cy.visit(`/k8s/cluster/nodes/${nodeName}/details`)
  },
  checkDetailsField: (fieldName, fieldValue) => {
    cy.get(`dt:contains('${fieldName}')`).next().contains(`${fieldValue}`);
  },
  checkChartURL: (chart: string, chartdetails?: RegExp) =>{
    cy.get(`[aria-label="View ${chart} metrics in query browser"]`)
      .should('have.attr','href')
      .and('match',chartdetails)
  },
  filterBy: (by, value) => {
    cy.get('[data-test-id="filter-dropdown-toggle"] button').click();
    cy.get('h1').contains(`${by}`).scrollIntoView();
    cy.get(`li[data-test-row-filter="${value}"] input`).check();
  },
  setAdditionalColumn: (columnName) => {
    cy.get('button[data-test="manage-columns"]').click();
    cy.get('form[name="form"]').should('be.visible');
    // restore default columns
    cy.contains('button', 'Restore default columns').click();
    // un-check Created column before selecting other columns
    cy.get('input[name="Created"]').uncheck();
    cy.get(`input[name="${columnName}"]`).check();
    cy.get('[data-test="confirm-action"]').click();
  },
  setDefaultColumn: () => {
    cy.get('button[data-test="manage-columns"]').click();
    cy.get('form[name="form"]').should('be.visible');
    cy.contains('button', 'Restore default columns').click();
    cy.get('[data-test="confirm-action"]').click();
  },
  checkMachineHealthCheck: (mhcName: string) => {
    cy.get('button[data-test="Health checks"]').click();
    cy.contains('a', `${mhcName}`).should('exist');
  }
}
