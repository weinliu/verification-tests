import { operatorHubPage } from "../../views/operator-hub-page";
import { listPage } from '../../upstream/views/list-page';

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc new-project test-ocp40457`);
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

  after(() => {
    const removeProject = ['test-ocp40457','test1-ocp56081', 'test2-ocp56081']
    cy.wrap(removeProject).each(projectName => {
      cy.adminCLI(`oc delete project ${projectName}`)
    })
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
    });

  it('(OCP-40457,yanpzhan) Install multiple operators in one project', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    operatorHubPage.installOperator('etcd', 'community-operators', 'test-ocp40457');
    operatorHubPage.installOperator('argocd-operator', 'community-operators', 'test-ocp40457');
    cy.visit(`/k8s/ns/test-ocp40457/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus('etcd', 'Succeed');
    operatorHubPage.checkOperatorStatus('Argo CD', 'Succeed');
    operatorHubPage.removeOperator('Argo CD', 'test-ocp40457');
    operatorHubPage.installOperator('cockroachdb', 'community-operators', 'test-ocp40457');
    cy.visit(`/k8s/ns/test-ocp40457/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus('CockroachDB Helm Operator', 'Succeed');
    });

  it('(OCP-56081),xiyuzhao) Check opt out when console deletes operands', {tags: ['e2e','admin', '@osd-ccs']}, () => {      
    //data preparation
    let testns
    const dataPreparation = (ns: string) => {
      cy.adminCLI(`oc new-project ${ns}`)
      operatorHubPage.installOperator('businessautomation-operator','redhat-operators', ns);
      cy.visit(`/k8s/ns/${ns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
      operatorHubPage.checkOperatorStatus('Business Automation', 'Succeed');
      cy.exec(`oc apply -f ./fixtures/operators/businessautomation-opreand.yaml -n ${ns}`)
        .then(result => expect(result.stdout).contains('created'));
    }
    
    //Uninstall Operator popsup window - Operand instances list and checkbox of 'Delete all operand' is added
    testns = 'test1-ocp56081'    //cy.adminCLI(`oc delete project test-ocp40457`);
    dataPreparation(testns);
    cy.visit(`/k8s/ns/${testns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);         
    listPage.rows.clickKebabAction(`Business Automation`,"Uninstall Operator");
    cy.contains('Operand instances')
      .as('checkText')
      .should('exist');
    cy.contains('a', /rhpam-trial/gi);
    cy.get('[name="delete-all-operands"]')
      .should('have.attr', 'data-checked-state', 'false')
      .click();
    cy.get('#confirm-action')
      .as('uninstallOperator')
      .click();
    cy.wait(10000);
    cy.exec(`oc get kieapp -n ${testns}`)
      .then(result => expect(result.stdout).contains(''));

    //When annotations disable-operand-delete = true, Operator will delete directly but leave Operand
    testns = 'test2-ocp56081'
    dataPreparation(testns);
    cy.visit(`/k8s/ns/${testns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`); 
    cy.exec(`oc get packagemanifests businessautomation-operator -n ${testns} -o yaml | grep "currentCSV:" | awk '{print $3}'`)
      .then((result) => {
        const currentCSV = result.stdout;
        cy.exec(`oc annotate csv ${currentCSV} -n ${testns} console.openshift.io/disable-operand-delete=true --overwrite`)
          .then(result => expect(result.stdout).contains('annotated'));
        });
    listPage.rows.clickKebabAction(`Business Automation`,"Uninstall Operator");
    cy.get('@checkText').should('not.exist');
    cy.get('@uninstallOperator').click();
    cy.exec(`oc get kieapp -n ${testns} | sed '1d' | awk '{print $1}'`)
      .its('stdout')
      .should('contain', 'rhpam-trial1')
      .should('contain', 'rhpam-trial2')
    });
      
})