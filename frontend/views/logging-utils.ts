import { listPage } from "../upstream/views/list-page";

//If specific channel/catsrc needed for testing, export the values using CYPRESS_EXTRA_PARAM before running the logging tests
//ex: export CYPRESS_EXTRA_PARAM='{"openshfift-logging": {"channel": "stable-5.7", "catalogsource": "qe-app-registry"}}'
const EXTRA_PARAM = JSON.stringify(Cypress.env("EXTRA_PARAM"))
const LOGGING_PARAM = (EXTRA_PARAM !== undefined) ? JSON.parse(EXTRA_PARAM) : null;

export const catalogSource = {
  //set channel
  channel: () => {
    let channel = (LOGGING_PARAM != null) ? LOGGING_PARAM['openshfift-logging']['channel'] : null;
    if(channel == null){
      channel = "stable";
    }
    return channel;
  },
  //set source namespace
  nameSpace: () => {
    let namespace = (LOGGING_PARAM != null) ? LOGGING_PARAM['openshfift-logging']['catsrc-namespace'] : null;
    if(namespace == null) {
      namespace = "openshift-marketplace";
    }
    return namespace;
  },
  //set source and check if the packagemanifest exists or not
  sourceName: () => {
    let csName = (LOGGING_PARAM != null) ? LOGGING_PARAM['openshfift-logging']['catalogsource'] : null;
    if(csName == null) {
     return catalogSource.qeCatSrc();
    } else {
      return cy.exec(`oc get catsrc ${csName} -n ${catalogSource.nameSpace()} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
      .then(result => {
        if(!result.stderr.includes('NotFound')) {
          return csName;
        } else {
          return "redhat-operators";
        }
      })
    }
  },
  qeCatSrc: () => {
    return cy.exec(`oc get catsrc -n openshift-marketplace qe-app-registry --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
    .then(result => {
        if(!result.stderr.includes('NotFound')) {
            return "qe-app-registry";
        } else {
            return "redhat-operators";
        }
    })
  }
};

export const logUtils = {
  installOperator: (namespace, packageName, csName, channelName?, enablePlugin?: boolean) => {
    cy.visit(`/operatorhub/subscribe?pkg=${packageName}&catalog=${csName}&catalogNamespace=openshift-marketplace&targetNamespace=undefined`);
    if (channelName){
      cy.get(`[data-test="${channelName}-radio-input"]`).click();
    }
    cy.exec(`oc get ns ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(result.stderr.includes('NotFound')){
        cy.get('input[data-test="enable-monitoring"]').click();
      } else {
        cy.contains('Namespace already exists').should('be.visible')
      }
    })
    if(enablePlugin){
      cy.get('input[name="logging-view-plugin"][data-test="Enable-radio-input"]').click();
    }
    cy.get('[data-test="install-operator"]').click();
  },
  uninstallOperator: (operatorName, nameSpace, packageName) => {
    cy.exec(`oc get sub ${packageName} -n ${nameSpace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
        cy.visit(`/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion`)    
        cy.byLegacyTestID(`resource-title`).should('be.visible')
        listPage.rows.clickKebabAction(`${operatorName}`,"Uninstall Operator");
        cy.get('#confirm-action').click();
        cy.get(`[data-test-operator-row="${operatorName}"]`).should('not.exist');
      }
    })
  }
};
