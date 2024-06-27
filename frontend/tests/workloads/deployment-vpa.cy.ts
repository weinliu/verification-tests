import { guidedTour } from "upstream/views/guided-tour";
import { testName } from '../../upstream/support';
import { Deployment } from 'views/deployment';
import { operatorHubPage } from 'views/operator-hub-page';
import { Pages } from "views/pages";
describe('deployment vpa related feature', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc create -f ./fixtures/deployments/ns-vpa.yaml`);
    cy.uiLogin(Cypress.env("LOGIN_IDP"),Cypress.env('LOGIN_USERNAME'),Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    operatorHubPage.installOperator('vertical-pod-autoscaler', 'redhat-operators', 'openshift-vertical-pod-autoscaler');
    cy.adminCLI(`oc new-project ${testName}`);
  });

  after(() => {
    cy.adminCLI(`oc delete project ${testName}`);
    cy.adminCLI('oc delete subscriptions.operators.coreos.com vertical-pod-autoscaler -n openshift-vertical-pod-autoscaler');
    cy.adminCLI('oc delete namespace openshift-vertical-pod-autoscaler');
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-68834,yanpzhan,UserInterface) Show recommended value for VerticalPodAutoscaler in the Admin deployments', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc create -f ./fixtures/deployments/exampledeployment-with-limits.yaml -n ${testName}`);
    //check vpa on deployment details page when no vpa
    cy.visit(`k8s/ns/${testName}/deployments/testd`);
    Deployment.checkDetailItem('VerticalPodAutoscaler', 'No VerticalPodAutoscaler');
    cy.wait(30000);
    Pages.gotoInstalledOperatorPage(`openshift-vertical-pod-autoscaler`)
    operatorHubPage.checkOperatorStatus(`VerticalPodAutoscaler`, 'Succeeded')
    cy.adminCLI(`oc create -f ./fixtures/deployments/testvpa.yaml -n ${testName}`);
    cy.adminCLI(`oc get verticalpodautoscaler -n ${testName}`).then(result => { expect(result.stdout).contain("examplevpa")})
    //check vpa on workload page
    cy.visit(`k8s/cluster/projects/${testName}/workloads?view=list`);
    cy.get('ul[aria-label="Deployment sub-resources"]').click();
    cy.contains('VerticalPodAutoscaler').should('exist');
    cy.contains('examplevpa').should('exist');
    //check vpa on deployment details page when vpa is created
    cy.visit(`k8s/ns/${testName}/deployments/testd`);
    const vpainfo = ['examplevpa','Recommended','Container name','CPU','Memory'];
    vpainfo.forEach(function (info) {
      Deployment.checkDetailItem('VerticalPodAutoscaler', `${info}`);
    });
  });
})

