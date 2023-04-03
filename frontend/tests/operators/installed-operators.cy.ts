import { listPage } from "upstream/views/list-page";
import { operatorHubPage } from "../../views/operator-hub-page";
import { guidedTour } from './../../upstream/views/guided-tour';

describe('Operators Installed page test', () => {
  const params ={
    'ns54975': 'ocp54975-project',
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
    cy.adminCLI(`oc label namespace openshift security.openshift.io/scc.podSecurityLabelSync=false --overwrite`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/enforce=privileged --overwrite`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/audit=privileged --overwrite`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/warn=privileged --overwrite`)
    cy.adminCLI(`oc create -f ./fixtures/operators/custom-catalog-source.json`);
    });

  after(() => {
    cy.adminCLI(`oc patch olmconfigs cluster --type=merge -p 'spec: {features: {disableCopiedCSVs: false}}'`)
    cy.adminCLI(`oc delete subscription ${params.sonarqube} -n openshift`);
    cy.adminCLI(`oc delete clusterserviceversion ${params.sonarqube}.v0.0.6 -n openshift`);
    cy.adminCLI(`oc delete subscription ${params.infinispan} -n openshift-operators`);
    cy.adminCLI(`oc delete clusterserviceversion ${params.infinispan}.v2.1.5 -n openshift-operators `)
    cy.adminCLI(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace`);
    cy.adminCLI(`oc label namespace openshift security.openshift.io/scc.podSecurityLabelSync-`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/enforce-`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/audit-`)
    cy.adminCLI(`oc label namespace openshift pod-security.kubernetes.io/warn-`)
    cy.adminCLI(`oc delete project ${params.ns54975}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
    });

  it('(OCP-54975,xiyuzhao) Check OCP console works when copied CSVs are disabled',{tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {

    operatorHubPage.installOperator(`${params.sonarqube}`, `${params.csName}`, 'openshift');
    operatorHubPage.installOperator(`${params.infinispan}`, `${params.csName}`);
    cy.visit(`/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
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
    cy.adminCLI(`oc patch olmconfigs cluster --type=merge -p 'spec: {features: {disableCopiedCSVs: true}}'`)
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.visit(`/k8s/ns/${params.ns54975}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    listPage.filter.byName('sonarqube');
    listPage.rows.shouldNotExist(`Sonarqube Operator`);
    listPage.filter.byName('infinispan');
    listPage.rows.shouldExist('Infinispan Operator').click();
    cy.get('[data-test-id="openshift"]').should("exist");
    cy.get('[data-test="label-key"]').should("contain.text","olm.copiedFrom");
    });

})
