import { operatorHubPage } from "../views/operator-hub-page"

export const knmstate = {
  namespace:   "openshift-nmstate",
  operatorName: "kubernetes-nmstate-operator",
  packageName: "Kubernetes NMState Operator",
};

export const knmstateUtils = {
  install: () => {
    let csName = "qe-app-registry";
    cy.visit(`/operatorhub/subscribe?pkg=${knmstate.operatorName}&catalog=${csName}&catalogNamespace=openshift-marketplace&targetNamespace=${knmstate.namespace}`);
    cy.byTestID('install-operator').click();
    cy.contains('View Operator', { timeout: 60000 }).should('be.visible');
    cy.visit(`/k8s/ns/${knmstate.namespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus(knmstate.packageName, 'Succeeded');
  },

  uninstall: () => {
    cy.exec(`oc get sub ${knmstate.operatorName} -n ${knmstate.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
        cy.visit(`/k8s/ns/${knmstate.namespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
        operatorHubPage.removeOperator(knmstate.packageName);
      }
    })
    cy.exec(`oc get project ${knmstate.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
        cy.exec(`oc delete project ${knmstate.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
      } 
    })
  },

  createNMStateInstace: () => {
    cy.visit(`/k8s/ns/${knmstate.namespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    cy.contains(knmstate.packageName).should('exist').invoke('attr', 'href').then(href => {
      cy.visit(href);
    })
    cy.byLegacyTestID('horizontal-link-NMState').click();
    cy.byTestID('item-create').click();
    cy.byTestID('create-dynamic-form').click();
    cy.byTestID('nmstate').should('exist');
    cy.byTestID('refresh-web-console', { timeout: 120000 }).should('exist');
    cy.reload(true);
  },

  deleteNMStateInstace: () => {
    cy.exec(`oc get nmstate -n ${knmstate.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('No resources found') && 
         !result.stderr.includes(`server doesn't have a resource type "nmstate"`)){
        cy.visit(`k8s/cluster/nmstate.io~v1~NMState/nmstate`);
        cy.byLegacyTestID('actions-menu-button').click();
        cy.byTestActionID('Delete NMState').click();
        cy.byTestID('confirm-action').click();
        cy.byTestID('refresh-web-console', { timeout: 60000 }).should('exist');
        cy.reload(true);
      }
    })
  },
  
};
