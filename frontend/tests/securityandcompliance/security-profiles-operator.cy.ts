import { metricsTab } from "views/metrics";
import { Pages } from "views/pages";
import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc delete seccompprofiles --all -n security-profiles-operator`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete selinuxprofiles --all -n security-profiles-operator`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete rawselinuxprofiles --all -n security-profiles-operator`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete mutatingwebhookconfiguration spo-mutating-webhook-configuration -n security-profiles-operator`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project security-profiles-operator`, { timeout: 60000, failOnNonZeroExit: false });
  });

  after(() => {
    cy.adminCLI(`oc delete seccompprofiles --all -n openshift-security-profiles`, { failOnNonZeroExit: true });
    cy.adminCLI(`oc delete selinuxprofiles --all -n openshift-security-profiles`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete rawselinuxprofiles --all -n openshift-security-profiles`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete mutatingwebhookconfiguration spo-mutating-webhook-configuration -n openshift-security-profiles`, { failOnNonZeroExit: true });
    cy.adminCLI(`oc delete project openshift-security-profiles`, { timeout: 600000, failOnNonZeroExit: true });
  });

  it('(OCP-50400,xiyuan,Security_and_Compliance) Install the Security Profiles Operator through GUI and check metrics on GUI',{tags:['@smoke','@e2e','admin','@osd-ccs','@rosa']}, () => {
    // intall security profiles operator
    const params = {
      ns: "openshift-security-profiles",
      operatorName: "Security Profiles Operator"
    }
    operatorHubPage.installOperatorWithRecomendNamespace('security-profiles-operator','qe-app-registry');
    cy.get('[aria-valuetext="Loading..."]').should('exist');
    Pages.gotoInstalledOperatorPage('openshift-security-profiles')
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeed');

    //patch to create a seccompprofile
    cy.adminCLI(`oc rollout status daemonset spod  -n openshift-security-profiles`,  { timeout: 100000, failOnNonZeroExit: true });
    cy.adminCLI(`oc patch spod spod --type=merge -p '{"spec":{"enableLogEnricher":true}}' -n openshift-security-profiles`, { failOnNonZeroExit: true });
    cy.adminCLI(`oc rollout status daemonset spod  -n openshift-security-profiles`,  { timeout: 240000, failOnNonZeroExit: true });
    cy.adminCLI(`oc wait --for=condition=READY=true sp/log-enricher-trace -n openshift-security-profiles`,  { timeout: 30000, failOnNonZeroExit: true });

    //check the metrics is available
    cy.visit(`/monitoring/query-browser?query0=security_profiles_operator_seccomp_profile_total`);
    cy.get('body').should('be.visible')
    metricsTab.checkMetricsLoadedWithoutError()
    cy.get('table[aria-label="query results table"]').should('exist');
  });
})
