import { Operand,operatorHubPage } from "views/operator-hub-page"
import { Pages } from "views/pages";

describe('Display All Namespace Operands for Global Operators', () => {
  let csvname;
  const params = {
    ns68180: 'testproject-68180',
    ns50153: 'testproject-50153',
    operatorName: 'Red Hat Streams for Apache Kafka',
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
    Pages.gotoInstalledOperatorPage();
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeeded');
    cy.adminCLI(`oc create -f ./fixtures/operators/amqstreams-opreand-kafka.yaml -n ${params.ns68180}`);
    cy.exec(`oc get clusterserviceversion -o custom-columns=NAME:.metadata.name --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | grep amq`, { timeout: 20000 })
      .then((result) => {
        csvname = result.stdout;
    })
  })

  after(() => {
    cy.adminCLI(`oc delete subscription ${params.operatorPkgName} -n openshift-operators`);
    cy.adminCLI(`oc delete clusterserviceversion ${csvname} -n openshift-operators`);
    cy.adminCLI(`oc delete project ${params.ns50153}`, { timeout: 120000 });
    cy.adminCLI(`oc delete project ${params.ns68180}`, { timeout: 120000 });
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`, { timeout: 60000 });
  })

  it('(OCP-68180,xiyuzhao,UserInterface) Check the sort function in Resources Tab in operator details page',{tags:['@userinterface','e2e','admin']},() => {
    cy.visit(`/k8s/ns/${params.ns68180}/clusterserviceversions/${csvname}/kafka.strimzi.io~v1beta2~Kafka/${params.crName}/resources`)
    Operand.sortAndVerifyColumn('Name');
    Operand.sortAndVerifyColumn('Status');
    Operand.sortAndVerifyColumn('Created');
    Operand.sortAndVerifyColumn('Kind');
  })

  it('(OCP-50153,xiyuzhao,UserInterface) - Display All Namespace Operands for Global Operators',{tags:['@userinterface','e2e','admin']}, () => {
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