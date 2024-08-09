import { listPage } from "upstream/views/list-page";
import { operatorHubPage } from "../../views/operator-hub-page";
import { guidedTour } from './../../upstream/views/guided-tour';
import { Pages } from "views/pages";

describe('Operators Installed page test', () => {
  const params ={
    'ns54975': 'ocp54975-project',
    'specialNs': 'openshift',
    'csName': 'custom-catalogsource',
    'sonarqube': 'sonarqube-operator',
    'infinispan': 'infinispan-operator'
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.createProject(params.ns54975);
    cy.switchPerspective('Administrator');
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc label namespace ${params.specialNs} security.openshift.io/scc.podSecurityLabelSync=false --overwrite`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/enforce=privileged --overwrite`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/audit=privileged --overwrite`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/warn=privileged --overwrite`);
    cy.adminCLI(`oc create -f ./fixtures/operators/custom-catalog-source.json`);
    Pages.gotoCatalogSourcePage();
    operatorHubPage.installOperator(params.sonarqube, params.csName, params.specialNs);
    cy.get('[aria-valuetext="Loading..."]', {timeout: 120000 }).should('exist');
    operatorHubPage.installOperator(params.infinispan, params.csName);
    cy.get('[aria-valuetext="Loading..."]', {timeout: 120000 }).should('exist');
    });

  after(() => {
    cy.adminCLI(`oc patch olmconfigs cluster --type=merge -p 'spec: {features: {disableCopiedCSVs: false}}'`);
    cy.adminCLI(`oc delete subscription ${params.sonarqube} -n ${params.specialNs}`);
    cy.adminCLI(`oc delete clusterserviceversion ${params.sonarqube}.v0.0.6 -n ${params.specialNs}`);
    cy.adminCLI(`oc delete subscription ${params.infinispan} -n openshift-operators`);
    cy.adminCLI(`oc delete clusterserviceversion ${params.infinispan}.v2.1.5 -n openshift-operators `);
    cy.adminCLI(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace`);
    cy.adminCLI(`oc label namespace ${params.specialNs} security.openshift.io/scc.podSecurityLabelSync-`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/enforce-`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/audit-`);
    cy.adminCLI(`oc label namespace ${params.specialNs} pod-security.kubernetes.io/warn-`);
    cy.adminCLI(`oc delete project ${params.ns54975}`);
    cy.adminCLI(`oc adm policy remove-role-from-user admin ${Cypress.env('LOGIN_USERNAME')} -n ${params.specialNs}`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`, {failOnNonZeroExit: false});
  });

  it('(OCP-54975,xiyuzhao,UserInterface) Check OCP console works when copied CSVs are disabled',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    Pages.gotoInstalledOperatorPage();
    operatorHubPage.checkOperatorStatus('Sonarqube Operator', 'Succeed');
    operatorHubPage.checkOperatorStatus('Infinispan Operator', 'Succeed');
    /* 1. Check the default value for the Flag is false
       2. Check When Flag = false，CSV have copy file in other namespace */
    let cpt;
    cy.window().then((win: any) => {
       cpt = win.SERVER_FLAGS.copiedCSVsDisabled;
       expect(JSON.stringify(cpt)).contain("false")
    });
    cy.adminCLI(`oc get csv -n default`)
      .then(result => { expect(result.stdout).contain("infinispan")})
    cy.adminCLI(`oc get csv -n openshift-console`)
      .then(result => { expect(result.stdout).contain("infinispan")})

    /*
    Check for normal user
    1. The Flag can take affect after OLMConfig being updated
    2. The Flag only take affect for the operator installed in All namespace
    3. When Flag = true, the golbal installed operator's CSV ns will update to 'openshit'
         And new lable 'olm.copiedFrom' is added for the Operator
    */
    const command = "oc get olmconfigs cluster -o jsonpath='{.status.conditions[0].status}'"
    const expectedOutput = "True"
    cy.adminCLI(`oc patch olmconfigs cluster --type=merge -p 'spec: {features: {disableCopiedCSVs: True}}'`);
    cy.checkCommandResult(command,expectedOutput, {retries: 5, interval: 15000})
      .then(() => {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
        cy.visit(`/k8s/cluster/user.openshift.io~v1~User/${Cypress.env('LOGIN_USERNAME')}/roles`)
        cy.get('[data-test="msg-box-title"]').should('contain.text','Restricted Access');
        Pages.gotoInstalledOperatorPage(params.ns54975);
        cy.get(`[data-test-id="resource-title"]`, { timeout: 15000 }).should('contain.text',"Installed Operators");
        listPage.filter.byName('sonarqube');
        listPage.rows.shouldNotExist(`Sonarqube Operator`);
        listPage.filter.byName('infinispan');
        listPage.rows.shouldExist('Infinispan Operator').click();
        cy.get('[data-test-id="openshift"]').should("exist");
        cy.get('[data-test="label-key"]').should("contain.text","olm.copiedFrom");
      })
  });

  it.skip('(OCP-65876,xiyuzhao,UserInterface) Non cluster-admin user should able to update the operator in Console',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc adm policy add-role-to-user admin ${Cypress.env('LOGIN_USERNAME')} -n ${params.specialNs}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
    cy.uiLogout();
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    /*' Scription' Tab is exist for the operator installed in the All namespace
       Page is Restricted Access */
    Pages.gotoInstalledOperatorPage(params.specialNs)
    cy.get('[data-test-operator-row="Infinispan Operator"]', {timeout: 120000 }).click();
    cy.get('[data-test-id="horizontal-link-Subscription"]')
      .as('subscriptionTab')
      .should('exist')
      .click();
    cy.get('[data-test="msg-box-title"]').should('contain.text','Restricted Access');
    /* Check 'Scription' Tab is visible to normal user for operator installed in specific namespace
       User who has authority is able to check the installPlan and CatalogSource info */
    Pages.gotoInstalledOperatorPage(params.specialNs)
    cy.get('[data-test-operator-row="Sonarqube Operator"]', {timeout: 120000 }).click();
    cy.get('@subscriptionTab').should('exist').click();
    cy.get('[title="InstallPlan"]')
      .next('a')
      .should('have.attr', 'href')
      .and('include', 'InstallPlan');
    cy.get('[title="CatalogSource"]')
      .next('a')
      .should('have.attr', 'href')
      .and('include', 'CatalogSource');
  });
})