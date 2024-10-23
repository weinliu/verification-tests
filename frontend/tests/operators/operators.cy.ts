import { operatorHubPage } from "../../views/operator-hub-page";
import { listPage } from '../../upstream/views/list-page';
import { Pages } from "views/pages";

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc new-project test-ocp40457`);
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc create -f ./fixtures/operators/custom-catalog-source.json`);
    Pages.gotoCatalogSourcePage();
  });

  after(() => {
    cy.adminCLI(`oc delete project test1-ocp56081`, { timeout: 240000, failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project test2-ocp56081`, { timeout: 240000, failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project test-ocp40457`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project test1-ocp68675`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project test2-ocp68675`, {  failOnNonZeroExit: false });
    cy.adminCLI(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-68675,xiyuzhao,UserInterface) Check Managed Namespaces field when OperatorGourp is set up',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa', '@level0']}, () => {
    const params = {
      ns1: "test1-ocp68675",
      ns2: "test2-ocp68675",
      operatorName: "Streams for Apache Kafka"
    }
    // Prepare data - install Operator AMQ into ns2 operator
    cy.adminCLI(`oc new-project ${params.ns1}`);
    cy.adminCLI(`oc new-project ${params.ns2}`);
    operatorHubPage.installOperator('amq-streams','redhat-operators',params.ns2);
    cy.get('[aria-valuetext="Loading..."]').should('exist');
    Pages.gotoInstalledOperatorPage();
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeed');
    // Operator did not exist in ns1 until the namespace was added to ns2's OperatorGroup
    Pages.gotoInstalledOperatorPage(params.ns1);
    cy.get(`[data-test-operator-row="${params.operatorName}"]`).should('not.exist');
    // Check 1. Install the Operator when ns1 is added to ns2's OperatorGroup automatically 2. the mute message in ns1
    cy.adminCLI(`oc get operatorgroup -n ${params.ns2} --no-headers -o custom-columns=OPERATORGROUP:.metadata.name`).then((result) => {
      const operatorgorup = result.stdout;
      cy.adminCLI(`oc patch operatorgroup ${operatorgorup} -n ${params.ns2} --type=merge -p '{"spec":{"targetNamespaces":["${params.ns1}","${params.ns2}"]}}'`);
    });
    cy.reload();
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeed');
    cy.get(`a[data-test="${params.ns1}"]`)
      .should('exist')
      .parents('td')
      .contains(`operator is running in ${params.ns2} but is managing this namespace`);
    cy.get(`[data-test-operator-row="${params.operatorName}"]`).click();
    cy.get('[data-test-section-heading="ClusterServiceVersion details"]')
      .should('exist')
      .parents('div')
      .contains(`operator is running in ${params.ns2} but is managing this namespace`);
    // Check Managed Namespaces column for ns2, '2 Namespaces' would be shown
    Pages.gotoInstalledOperatorPage(params.ns2);
    cy.contains('button', '2 Namespaces')
      .should('be.visible')
      .click()
      .then(() => {
        cy.get(`[data-test-id="${params.ns1}"]`).should('exist');
        cy.get(`[data-test-id="${params.ns2}"]`).should('exist');
      })
  });
  it('(OCP-40457,yanpzhan,UserInterface) Install multiple operators in one project',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    operatorHubPage.installOperator('etcd', 'community-operators', 'test-ocp40457');
    cy.wait(20000);
    operatorHubPage.installOperator('argocd-operator', 'custom-catalogsource', 'test-ocp40457');
    Pages.gotoInstalledOperatorPage('test-ocp40457')
    operatorHubPage.checkOperatorStatus('etcd', 'Succeed');
    operatorHubPage.checkOperatorStatus('Argo CD', 'Succeed');
    operatorHubPage.removeOperator('Argo CD');
    operatorHubPage.installOperator('cockroachdb', 'community-operators', 'test-ocp40457');
    Pages.gotoInstalledOperatorPage('test-ocp40457')
    operatorHubPage.checkOperatorStatus('CockroachDB Helm Operator', 'Succeed');
  });

  it('(OCP-56081,xiyuzhao,UserInterface) Check opt out when console deletes operands',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    const testParams = {
      ns1: "test1-ocp56081",
      ns2: "test2-ocp56081",
      operatorName: "Business Automation",
      subscriptionName: "businessautomation-operator"
    }
    const uninstallOperatorCheckOperand = (ns: string, condition: string) => {
      Pages.gotoInstalledOperatorPage(ns);
      listPage.rows.clickKebabAction(testParams.operatorName,"Uninstall Operator");
      cy.contains('Operand instances').should(condition);
    };

    //data preparation
    const testns = [testParams.ns1,testParams.ns2]
    cy.wrap(testns).each(ns => {
      cy.adminCLI(`oc new-project ${ns}`)
      operatorHubPage.installOperator(testParams.subscriptionName,'redhat-operators', ns);
      cy.get('[aria-valuetext="Loading..."]').should('exist');
      Pages.gotoInstalledOperatorPage(`${ns}`);
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
          .should('contain','annotate');
        });
    uninstallOperatorCheckOperand(testParams.ns2, 'not.exist');
    cy.get('@uninstallOperator').click();
    cy.adminCLI(`oc get kieapp -n ${testParams.ns2}`)
      .its('stdout')
      .should('contain', 'rhpam-trial1')
      .should('contain', 'rhpam-trial2');
  });
})
