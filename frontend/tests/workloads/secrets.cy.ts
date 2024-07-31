import { testName } from "../../upstream/support";
import { Secrets } from "../../views/secrect";
import { Pages } from "views/pages";
import { listPage } from "../../upstream/views/list-page";
import { detailsPage } from "../../upstream/views/details-page";
import { guidedTour } from '../../upstream/views/guided-tour';

let project_name = testName;
describe('Workload Secrets test', () => {
  before(() => {
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.createProject(project_name);
    cy.adminCLI(`oc create -f ./fixtures/secret-tls.yaml -n ${project_name}`);
    cy.adminCLI(`oc create secret generic test1 -n ${project_name} --from-file=data1=./fixtures/original.jks`)
    cy.adminCLI(`oc get secret -n ${project_name} test1 -o yaml > /tmp/secret1.yaml`)
  });

  after(() => {
    cy.adminCLI(`oc delete project ${project_name}`);
  });

  it('(OCP-47010,xiyuzhao,UserInterface) Check Secrets is editable on console', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    Secrets.gotoSecretsPage(project_name);
    listPage.filter.byName('tlssecret');
    listPage.rows.countShouldBe(1);

    listPage.rows.clickKebabAction('tlssecret','Edit Secret')
    cy.url().should('include',`/tlssecret/edit`);

    Secrets.addKeyValue("keyfortest", "valuefortest");
    cy.get('#save-changes').click();
    detailsPage.isLoaded();
    Secrets.validKeyValueExist("keyfortest", "valuefortest");
  });

  it('(OCP-54014,xiyuzhao,UserInterface) Check Base64 data value for jave keystore secret would not change without changing anything', {tags: ['e2e','admin']}, () => {
    cy.visit(`/k8s/ns/${project_name}/secrets/test1/edit`)
    cy.byTestID('save-changes').click()
    cy.exec(`oc get secret -n ${project_name} test1 -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} > /tmp/secret2.yaml`)
    cy.exec(`diff /tmp/secret1.yaml /tmp/secret2.yaml`)
      .its('stdout')
      .should('eq', '')
  });

  it('(OCP-54213,yanpzhan,UserInterface) Trim whitespace to form inputs when create image pull secret', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    guidedTour.close();
    Secrets.gotoSecretsPage(project_name);
    Secrets.createImagePullSecret('secrettest','  quay.io  ','  testuser  ','  testpassword  ','  test@redhat.com  ');
    Secrets.revealValue();
    cy.get('code').should('contain','{"auths":{"quay.io":{"username":"testuser","password":"testpassword","auth":"dGVzdHVzZXI6dGVzdHBhc3N3b3Jk","email":"test@redhat.com"}}}');
  });

  it('(OCP-73150,yapei)Passwords entered on the console are obfuscated', {tags: ['e2e','@osd-ccs','@rosa', '@wrs']}, () => {
    // input[type="password"] will make characters masked
    Pages.gotoImagePullSecretCreation(project_name);
    cy.get('input[data-test="image-secret-password"]')
      .should('have.attr', 'type', 'password');
    Pages.gotoSourceSecretCreation(project_name);
    cy.get('input[data-test="secret-password"]')
      .should('have.attr', 'type', 'password');
  });
})
