export const nodesPage = {
  goToNodesPage: () => {
    cy.visit('/k8s/cluster/core~v1~Node').get('[data-test-id="resource-title"]').should('be.visible')
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
  setAdditionalColumn: (columnName) => {
    cy.get('button[data-test="manage-columns"]').click();
    cy.get('form[name="form"]').should('be.visible');
    cy.get('input[name="Created"]').click();
    cy.get('input[name="Uptime"]').click();  
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
