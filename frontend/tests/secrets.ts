import { testName } from "../upstream/support";
import { Secrets } from "../views/secrect";
import { listPage } from "../upstream/views/list-page";
import { detailsPage } from "../upstream/views/details-page";
import { guidedTour } from '../upstream/views/guided-tour';

describe('Workload Secrets test', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject(testName);
    cy.exec(`oc create -f ./fixtures/secret-tls.yaml -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);   
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
})