import { Operand,operatorHubPage } from "views/operator-hub-page"

describe('Display All Namespace Operands for Global Operators', () => {
  let csvname;
  const params = {
    ns68180: 'testproject-68180',
    ns50153: 'testproject-50153',
    operatorName: 'AMQ Streams',
    operatorPkgName: "amq-streams",
    crName: 'kfakasample'
  }
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project ${params.ns68180}`);
    cy.adminCLI(`oc new-project ${params.ns50153}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
    // Base data for case OCP-68180 & OCP-50153
    operatorHubPage.installOperator(params.operatorPkgName, "redhat-operators");
    cy.get('[aria-valuetext="Loading..."]').should('exist');
    cy.visit('/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion');
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeeded');
    cy.adminCLI(`oc create -f ./fixtures/operators/amqstreams-opreand-kafka.yaml -n ${params.ns68180}`);
    cy.adminCLI(`oc get clusterserviceversion -o=jsonpath='{.items[*].metadata.name}'`).then((result) => {
      csvname = result.stdout;
    })
  })

  after(() => {
    cy.adminCLI(`oc delete subscription ${params.operatorPkgName} -n openshift-operators`);
    cy.adminCLI(`oc delete clusterserviceversion ${csvname} -n openshift-operators`);
    cy.adminCLI(`oc delete project ${params.ns50153}`);
    cy.adminCLI(`oc delete project ${params.ns68180}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  })

  it('(OCP-68180,xiyuzhao,UserInterface) Check the sort function in Resources Tab in operator details page', {tags: ['e2e','admin']},() => {
    cy.visit(`/k8s/ns/${params.ns68180}/clusterserviceversions/${csvname}/kafka.strimzi.io~v1beta2~Kafka/${params.crName}/resources`)
    Operand.sortAndVerifyColumn('Name');
    Operand.sortAndVerifyColumn('Status');
    Operand.sortAndVerifyColumn('Created');
    Operand.sortAndVerifyColumn('Kind');
  })

  it('(OCP-50153,xiyuzhao,UserInterface) - Display All Namespace Operands for Global Operators', {tags: ['e2e','admin']}, () => {
    cy.visit(`/k8s/ns/${params.ns50153}/operators.coreos.com~v1alpha1~ClusterServiceVersion/${csvname}/instances`)
    // checkpoint 1: Check column 'Namespace' is added in list
    cy.get('[data-label="Namespace"]').should('be.visible');
    // checkpoint 2: Check 'All namespace' radio input is selected by dafault
    cy.byTestID('All namespaces-radio-input').should('be.checked');
    // checkpoint 3: Check subscription is listed
    cy.byTestID(params.crName).should('be.visible');
    // checkpoint 4: Check only corresponding resource is displayed on specific ns
    cy.byTestID('Current namespace only-radio-input').click();
    cy.byTestID(params.crName).should('not.exist');
  })
})