export const nodesPage = {
  goToNodesPage: () => {
    cy.visit('/k8s/cluster/core~v1~Node').get('[data-test-id="resource-title"]').should('be.visible')
  },
  gotoDetail: (nodeName) => { cy.visit(`/k8s/cluster/nodes/${nodeName}/details`) },
  checkDetailsField: (fieldName, fieldValue) => {
    cy.get(`dt:contains('${fieldName}')`).next().contains(`${fieldValue}`);
  }
}
