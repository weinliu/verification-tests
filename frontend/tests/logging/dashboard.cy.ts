import { catalogSource, logUtils } from "../../views/logging-utils";
import { dashboard, graphSelector } from "views/dashboards-page"
describe('Logging related features', () => {
  const CLO = {
    Namespace:   "openshift-logging",
    PackageName: "cluster-logging"
  }
  const EO = {
    Namespace:   "openshift-operators-redhat",
    PackageName: "elasticsearch-operator"
  }
  const LO = {
    Namespace:   "openshift-operators-redhat",
    PackageName: "loki-operator"
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    // Install logging operators if needed
    catalogSource.sourceName().then((csName) => {
      logUtils.installOperator(CLO.Namespace, CLO.PackageName, csName, catalogSource.channel(), true);
      logUtils.installOperator(EO.Namespace, EO.PackageName, csName, catalogSource.channel());
      logUtils.installOperator(LO.Namespace, LO.PackageName, csName, catalogSource.channel());
    });
  });

  after(() => {
    logUtils.removeClusterLogging(CLO.Namespace)
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-53071,qitang) Vector - OpenShift Logging Collection Vector metrics dashboard', {tags: ['e2e','admin']}, () => {
    // Create Logging instance
    logUtils.createClusterLoggingViaYamlView(CLO.Namespace, "cl_default_es.yaml")

    logUtils.waitforPodReady(CLO.Namespace, 'component=elasticsearch');
    logUtils.waitforPodReady(CLO.Namespace, 'component=kibana');
    logUtils.waitforPodReady(CLO.Namespace, 'component=collector');

    dashboard.visitDashboard('grafana-dashboard-cluster-logging');

    cy.byLegacyTestID('panel-overview').should('exist').within(() => {
      cy.byTestID('total-log-bytes-collected-last-24h-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state');
      cy.byTestID('total-log-bytes-sent-last-24h-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('rate-log-bytes-collected-last-24h-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('rate-log-bytes-sent-last-24h-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('total-errors-last-60m-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-outputs').should('exist').within(() => {
        cy.byTestID('rate-log-bytes-sent-per-output-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-produced-logs').should('exist').within(() => {
        cy.byTestID('top-producing-containers-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        cy.byTestID('top-producing-containers-in-last-24-hours-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-machine').should('exist').within(() => {
      cy.byTestID('cpu-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('memory-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('running-containers-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('open-files-for-container-logs-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });
  });

});
