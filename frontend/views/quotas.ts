export const quotaPage = {
  goToOneProjectQuotaPage: (projectname, quotaname) => cy.visit(`/k8s/ns/${projectname}/resourcequotas/${quotaname}`),
  checkResourceQuotaListed: (resourcename) => cy.contains('h5',`${resourcename}`).should('exist')
}
