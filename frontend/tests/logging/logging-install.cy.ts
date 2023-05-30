import { catalogSource, logUtils } from "../../views/logging-utils";

//ex: export CYPRESS_EXTRA_PARAM='{"openshfift-logging": {"channel": "stable-5.7", "catalogsource": "qe-app-registry"}}' before running logging tests if required
describe('Logging related features', () => {
  const CLO = {
    Namespace:   "openshift-logging",
    PackageName: "cluster-logging"
  };
  const EO = {
    Namespace:   "openshift-operators-redhat",
    PackageName: "elasticsearch-operator"
  };
  const LOKI = {
    Namespace:     "openshift-operators-redhat",
    PackageName:   "loki-operator",
  };

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    //Delete logging operators if already exists
    logUtils.uninstallOperator('Red Hat OpenShift Logging', CLO.Namespace, CLO.PackageName);
    logUtils.uninstallOperator('OpenShift Elasticsearch Operator', EO.Namespace, EO.PackageName);
    logUtils.uninstallOperator('Loki Operator', LOKI.Namespace, LOKI.PackageName);
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-22558,gkarager) Deploy cluster-logging operator via web console', {tags: ['e2e','admin']}, () => {
    //Install the Cluster Logging Operator with console plungin
    catalogSource.sourceName().then((csName) => {
      logUtils.installOperator(CLO.Namespace, CLO.PackageName, csName, catalogSource.channel(), true);
    });
    cy.contains('View Operator').should('be.visible');
  });

  it('(OCP-24292,gkarager) Deploy elasticsearch-operator via Web Console', {tags: ['e2e','admin']}, () => {
    //Install the Elasticsearch Operator
    catalogSource.sourceName().then((csName) => {
      logUtils.installOperator(EO.Namespace, EO.PackageName, csName, catalogSource.channel());
    });
    cy.contains('View Operator').should('be.visible');
  });

  it('(gkarager) Deploy loki-operator via Web Console', {tags: ['e2e','admin']}, () => {
    //Install the Loki Operator
    catalogSource.sourceName().then((csName) => {
      logUtils.installOperator(LOKI.Namespace, LOKI.PackageName, csName, catalogSource.channel());
    });
    cy.contains('View Operator').should('be.visible');
  });
});
