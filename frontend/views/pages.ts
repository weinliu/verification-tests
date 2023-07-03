import { listPage } from "upstream/views/list-page";

export const Pages = {
  gotoProjectsList: () => {
    cy.visit('/k8s/cluster/project.openshift.io~v1~Project');
    listPage.rows.shouldBeLoaded();
  },
  gotoNamespacesList: () => {
    cy.visit('/k8s/cluster/core~v1~Namespace');
    listPage.rows.shouldBeLoaded();
  },
  gotoSearch: () => {
    cy.visit('/search/all-namespaces');
    cy.get('.pf-c-toolbar__content-section').should('be.visible');
  },
  gotoAPIExplorer: () => {
    cy.visit('/api-explorer');
    cy.get('table[aria-label="API resources"]').should('be.visible');
  },
  gotoDeploymentsList: () => {
    cy.visit('/k8s/all-namespaces/apps~v1~Deployment');
    listPage.rows.shouldBeLoaded();
  },
  gotoClusterOperatorsList: () => {
    cy.visit('/settings/cluster/clusteroperators');
    listPage.rows.shouldBeLoaded();
  },
  gotoCRDsList: () => {
    cy.visit('/k8s/cluster/apiextensions.k8s.io~v1~CustomResourceDefinition');
    listPage.rows.shouldBeLoaded();
  }
}