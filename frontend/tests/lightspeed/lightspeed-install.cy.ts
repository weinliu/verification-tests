import { operatorHubPage } from "../../views/operator-hub-page";
import { Pages } from "../../views/pages";

describe('Lightspeed related features', () => {
  const OLS = {
    namespace:   "openshift-lightspeed",
    packageName: "lightspeed-operator",
    operatorName: "OpenShift Lightspeed Operator",
    config: {
      kind: 'OLSConfig',
      name: 'cluster',
    },
  };

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    // Delete entire namespace to delete operator and ensure everything else is cleaned up
    cy.adminCLI(`oc delete namespace ${OLS.namespace}`, { failOnNonZeroExit: false });

    // Delete config
    cy.exec(`oc delete ${OLS.config.kind} ${OLS.config.name} -n ${OLS.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });

    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OLS-427,jfula,Lightspeed) Deploy OpenShift Lightspeed operator via web console', {tags: ['e2e','admin', '@smoke']}, () => {
    operatorHubPage.installOperator(OLS.packageName, "redhat-operators");
    cy.get('.co-clusterserviceversion-install__heading', { timeout: 2 * 60 * 1000 }).should('include.text', 'ready for use');

    const config = `apiVersion: ols.openshift.io/v1alpha1
kind: ${OLS.config.kind}
metadata:
  name: ${OLS.config.name}
spec:
  llm:
    providers:
      - type: openai
        name: openai
        credentialsSecretRef:
          name: openai-api-keys
        url: https://api.openai.com/v1
        models:
          - name: gpt-3.5-turbo
  ols:
    defaultModel: gpt-3.5-turbo
    defaultProvider: openai
    logLevel: INFO`;
    cy.exec(`echo '${config}' | oc create -f -`);

    //Install the OpenShift Lightspeed catalog source
    // lightUtils.installOperator(OLS.namespace, OLS.packageName, "redhat-operators", catalogSource.channel(OLS.packageName), catalogSource.version(OLS.packageName), false, OLS.operatorName);
    //Install the OpenShift Lightspeed Operator with console plugin
  });
});
