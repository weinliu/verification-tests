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
  gotoCreateDeploymentFormView: (namespace: string) => {
    cy.visit(`/k8s/ns/${namespace}/deployments/~new/form`);
    cy.get('[data-test="form-view-input"]').click({force: true});
    cy.get('[data-test="section name"]').should("exist");
  },
  gotoCreateDeploymentYamlView: (namespace: string) => {
    cy.visit(`/k8s/ns/${namespace}/deployments/~new/form`);
    cy.get('[data-test="yaml-view-input"]').click({force: true});
    cy.get('[data-test="yaml-editor"]').should("exist");
  },
  gotoCreateDeploymentconfigsFormView: (namespace: string) => {
    cy.visit(`/k8s/ns/${namespace}/deploymentconfigs/~new/form`);
    cy.get('[data-test="form-view-input"]').click({force: true});
    cy.get('[data-test="section name"]').should("exist");
  },
  gotoCreateDeploymentconfigsYamlView: (namespace: string) => {
    cy.visit(`/k8s/ns/${namespace}/deploymentconfigs/~new/form`);
    cy.get('[data-test="yaml-view-input"]').click({force: true});
    cy.get('[data-test="yaml-editor"]').should("exist");
  },
  gotoDeploymentsList: () => {
    cy.visit('/k8s/all-namespaces/apps~v1~Deployment');
    listPage.rows.shouldBeLoaded();
  },
  gotoDeploymentConfigList: (namespace: string) => {
    cy.visit(`/k8s/ns/${namespace}/apps.openshift.io~v1~DeploymentConfig`);
  },
  gotoDeploymentConfigDetailsTab: (namespace: string, dcname: string)=> {
    cy.visit(`/k8s/ns/${namespace}/deploymentconfigs/${dcname}`);
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