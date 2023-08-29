import { operatorHubPage } from "../../views/operator-hub-page";
import { listPage } from '../../upstream/views/list-page';

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc new-project test-ocp40457`);
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.exec(`oc delete project test1-ocp56081 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {timeout: 240000, failOnNonZeroExit: false})
    cy.exec(`oc delete project test2-ocp56081 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {timeout: 240000, failOnNonZeroExit: false})
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc delete project test-ocp40457`);
  });

  it('(OCP-40457,yanpzhan) Install multiple operators in one project', {tags: ['e2e','admin','@osd-ccs','@rosa','@smoke']}, () => {
    operatorHubPage.installOperator('etcd', 'community-operators', 'test-ocp40457');
    operatorHubPage.installOperator('argocd-operator', 'community-operators', 'test-ocp40457');
    cy.visit(`/k8s/ns/test-ocp40457/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus('etcd', 'Succeed');
    operatorHubPage.checkOperatorStatus('Argo CD', 'Succeed');
    operatorHubPage.removeOperator('Argo CD');
    operatorHubPage.installOperator('cockroachdb', 'community-operators', 'test-ocp40457');
    cy.visit(`/k8s/ns/test-ocp40457/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus('CockroachDB Helm Operator', 'Succeed');
  });

  it('(OCP-56081),xiyuzhao) Check opt out when console deletes operands', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    const testParams = {
      ns1: "test1-ocp56081",
      ns2: "test2-ocp56081",
      operatorName: "Business Automation",
      subscriptionName: "businessautomation-operator"
    }
    const uninstallOperatorCheckOperand = (ns: string, condition: string) => {
      cy.visit(`/k8s/ns/${ns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
      listPage.rows.clickKebabAction(testParams.operatorName,"Uninstall Operator");
      cy.contains('Operand instances').should(condition);
    };

    //data preparation
    const testns = [testParams.ns1,testParams.ns2]
    cy.wrap(testns).each(ns => {
      cy.adminCLI(`oc new-project ${ns}`)
      operatorHubPage.installOperator(testParams.subscriptionName,'redhat-operators', ns);
      cy.get('[aria-valuetext="Loading..."]').should('exist');
      cy.visit(`/k8s/ns/${ns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
      operatorHubPage.checkOperatorStatus(testParams.operatorName, 'Succeed');
      cy.adminCLI(`oc apply -f ./fixtures/operators/businessautomation-opreand.yaml -n ${ns}`)
        .its('stdout')
        .should('contain','created');
    });

    //Uninstall Operator popsup window - Operand instances list and checkbox of 'Delete all operand' is added
    uninstallOperatorCheckOperand(testParams.ns1, 'exist');
    //Add if-else to solve the issue that the operand list fails to load, causing case failure in CI.
    cy.get('.loading-box')
      .then(($el) => {
        if ($el.find('[data-test-operand-kind="KieApp"]').length) {
          cy.contains('a', /rhpam-trial/gi);
        } else {
          uninstallOperatorCheckOperand(testParams.ns1, 'exist');
          cy.contains('a', /rhpam-trial/gi);
        }
      });
    cy.get('[name="delete-all-operands"]')
      .should('have.attr', 'data-checked-state', 'false')
      .click();
    cy.get('#confirm-action')
      .as('uninstallOperator')
      .click();
    cy.byTestID('confirm-action').should('be.disabled')
    cy.adminCLI(`oc get kieapp -n ${testParams.ns1}`)
      .its('stdout')
      .should('be.empty');

    //When annotations disable-operand-delete = true, Operator will delete directly but leave Operand
    cy.adminCLI(`oc get subscriptions ${testParams.subscriptionName} -n ${testParams.ns2} -o jsonpath='{.status.currentCSV}'`)
      .then((result) => {
        const currentCSV = result.stdout;
        cy.adminCLI(`oc annotate csv ${currentCSV} -n ${testParams.ns2} console.openshift.io/disable-operand-delete=true --overwrite`)
          .its('stdout')
          .should('contain','annotated');
        });
    uninstallOperatorCheckOperand(testParams.ns2, 'not.exist');
    cy.get('@uninstallOperator').click();
    cy.adminCLI(`oc get kieapp -n ${testParams.ns2}`)
      .its('stdout')
      .should('contain', 'rhpam-trial1')
      .should('contain', 'rhpam-trial2');
  });
})