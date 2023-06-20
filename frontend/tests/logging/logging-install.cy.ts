import { catalogSource, logUtils } from "../../views/logging-utils";

describe('Logging related features', () => {
  const CLO = {
    namespace:   "openshift-logging",
    packageName: "cluster-logging"
  };
  const EO = {
    namespace:   "openshift-operators-redhat",
    packageName: "elasticsearch-operator"
  };
  const LO = {
    namespace:   "openshift-operators-redhat",
    packageName: "loki-operator"
  };

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    //Delete logging operators if already exists
    logUtils.uninstallOperator('Red Hat OpenShift Logging', CLO.namespace, CLO.packageName);
    logUtils.uninstallOperator('OpenShift Elasticsearch Operator', EO.namespace, EO.packageName);
    logUtils.uninstallOperator('Loki Operator', LO.namespace, LO.packageName);
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-22558,gkarager) Deploy cluster-logging operator via web console', {tags: ['e2e','admin']}, () => {
    //Install the Cluster Logging Operator with console plungin
    catalogSource.sourceName(CLO.packageName).then((csName) => {
      logUtils.installOperator(CLO.namespace, CLO.packageName, csName, catalogSource.channel(CLO.packageName), true);
    });
    cy.contains('View Operator').should('be.visible');
  });

  it('(OCP-24292,gkarager) Deploy elasticsearch-operator via Web Console', {tags: ['e2e','admin']}, () => {
    //Install the Elasticsearch Operator
    catalogSource.sourceName(EO.packageName).then((csName) => {
      logUtils.installOperator(EO.namespace, EO.packageName, csName, catalogSource.channel(EO.packageName));
    });
    cy.contains('View Operator').should('be.visible');
  });

  it('(gkarager) Deploy loki-operator via Web Console', {tags: ['e2e','admin']}, () => {
    //Install the Loki Operator
    catalogSource.sourceName(LO.packageName).then((csName) => {
      logUtils.installOperator(LO.namespace, LO.packageName, csName, catalogSource.channel(LO.packageName));
    });
    cy.contains('View Operator').should('be.visible');
  });
});
