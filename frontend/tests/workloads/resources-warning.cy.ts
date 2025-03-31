import { listPage } from "../../upstream/views/list-page";
import { testName } from '../../upstream/support';
import { Deployment } from "views/deployment";
import { Pages } from "views/pages";
import { guidedTour } from '../../upstream/views/guided-tour';
import { importYamlPage } from "views/yaml-page";

describe('show warning info for resources', () => {
  before(() => {
    cy.cliLogin();
    cy.adminCLI(`oc apply -f ./fixtures/warning-policy/gatekeeper.yaml`, {failOnNonZeroExit: false});
    cy.checkCommandResult(`oc get pods -n gatekeeper-system`, 'Running');
    cy.adminCLI(`oc create -f ./fixtures/warning-policy/gatekeeper-template.yaml`, {failOnNonZeroExit: false});
    cy.wait(5000);
    cy.adminCLI(`oc get crd k8srequiredlabels.constraints.gatekeeper.sh`, {failOnNonZeroExit: false})
      .then(output => {
         expect(output.stdout).contain('CREATED');
    });
    cy.adminCLI(`oc create -f ./fixtures/warning-policy/podrequiredlabel.yaml`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc create -f ./fixtures/warning-policy/deployrequiredlabel.yaml`, {failOnNonZeroExit: false});
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.createProject(testName);
  });

  after(() => {
    cy.cliLogout();
    cy.adminCLI(`oc delete -f ./fixtures/warning-policy/gatekeeper.yaml`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc delete k8srequiredlabels.constraints.gatekeeper.sh --all`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc delete project ${testName}`, {failOnNonZeroExit: false});
  });
  it('(OCP-73390,yanpzhan,UserInterface) Display a warning message from kube-apiserver when creating/updating workloads resources for create/import yaml editor',{tags:['@userinterface','@e2e','@rosa','@osd-ccs']}, () => {
    const WARNING_FOO = 'pod-must-have-label-foo';
    const WARNING_BAR = 'deploy-must-have-label-bar';
    listPage.createNamespacedResourceWithDefaultYAML('core~v1~Pod', `${testName}`);
    cy.contains(`${WARNING_FOO}`).should('exist');
    cy.contains('Learn more').should('exist');
    Pages.addLabelFromResourcePage('test=one');
    cy.contains(`${WARNING_FOO}`).should('exist');

    Deployment.createDeploymentFromForm(`${testName}`, 'testd');
    cy.contains(`${WARNING_BAR}`).should('exist');
    Pages.addLabelFromResourcePage('testd=one');
    cy.contains(`${WARNING_BAR}`).should('exist');

    cy.visit(`/k8s/cluster/projects/${testName}/yaml`);
    importYamlPage.open();
    cy.wait(3000);
    cy.get('.view-line')
        .selectFile('./fixtures/warning-policy/multiple-import.yaml', {action: 'drag-drop'});
    cy.byTestID('save-changes').should('exist');
    cy.wait(2000);
    cy.byTestID('save-changes').click({force: true});
    cy.contains(`${WARNING_FOO}`).should('exist');
    cy.contains(`${WARNING_BAR}`).should('exist');

  })
})

