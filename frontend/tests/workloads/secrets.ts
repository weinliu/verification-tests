import { testName } from "../../upstream/support";
import { Secrets } from "../../views/secrect";
import { listPage } from "../../upstream/views/list-page";
import { detailsPage } from "../../upstream/views/details-page";
import { guidedTour } from '../../upstream/views/guided-tour';

describe('Workload Secrets test', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject(testName);
    cy.exec(`oc create -f ./fixtures/secret-tls.yaml -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);   
    cy.exec(`oc create secret generic test1 -n ${testName} --from-file=data1=./fixtures/original.jks --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc get secret -n ${testName} test1 -o yaml > ./fixtures/secret1.yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
  });

  after(() => {
    cy.exec(`oc delete project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.logout();
  });

  it('OCP-47010 - Check Secrets is editable on console', () => {
    Secrets.gotoSecretsPage(testName);
    listPage.filter.byName('tlssecret');
    listPage.rows.countShouldBe(1);

    listPage.rows.clickKebabAction('tlssecret','Edit Secret')
    cy.url().should('include',`/tlssecret/edit`);
    
    Secrets.addKeyValue("keyfortest", "valuefortest");
    cy.get('#save-changes').click();
    detailsPage.isLoaded();
    Secrets.validKeyValueExist("keyfortest", "valuefortest");
  });

  it('(OCP-54014, xiangyli) Check Base64 data value for jave keystore secret would not change without changing anything', () => {
    cy.visit(`/k8s/ns/${testName}/secrets/test1/edit`)
    cy.byTestID('save-changes').click()
    cy.exec(`oc get secret -n ${testName} test1 -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} > ./fixtures/secret2.yaml`)
    cy.exec(`diff ./fixtures/secret1.yaml ./fixtures/secret2.yaml`)
      .its('stdout')
      .should('eq', '')
  });

  it('(OCP-54213,yanpzhan) Trim whitespace to form inputs when create image pull secret', () => {
    guidedTour.close();
    Secrets.gotoSecretsPage(testName);
    Secrets.createImagePullSecret('secrettest','  quay.io  ','  testuser  ','  testpassword  ','  test@redhat.com  ');
    Secrets.revealValue();
    cy.get('code').should('contain','{"auths":{"quay.io":{"username":"testuser","password":"testpassword","auth":"dGVzdHVzZXI6ICB0ZXN0cGFzc3dvcmQgIA==","email":"test@redhat.com"}}}');
  });
})
