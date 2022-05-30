export const Insights = {
  openInsightsPopup: () => cy.get('button[data-test="Insights"]', { timeout: 60000 }).click(),
  checkSeverityLinks: (clusterid) => {
    cy.get('g a').eq(0).should('have.attr', 'href').and('contain', `insights/advisor/clusters/${clusterid}?total_risk=4`);
    cy.get('g a').eq(1).should('have.attr', 'href').and('contain', `insights/advisor/clusters/${clusterid}?total_risk=3`);
    cy.get('g a').eq(2).should('have.attr', 'href').and('contain', `insights/advisor/clusters/${clusterid}?total_risk=2`);
    cy.get('g a').eq(3).should('have.attr', 'href').and('contain', `insights/advisor/clusters/${clusterid}?total_risk=1`);
  },
  checkLinkForInsightsAdvisor: (clusterid) => {
    cy.contains('Fixable issues').next().find('a').should('have.attr', 'href').and('contain', `insights/advisor/clusters/${clusterid}`);
  }
}
