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
  }
}