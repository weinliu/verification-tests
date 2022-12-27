import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features', () => {
  before(() => {
    cy.exec(`oc new-project test-ocp40457 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

  after(() => {
    cy.exec(`oc delete project test-ocp40457 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.logout;
    });

  it('(OCP-40457,yanpzhan) Install multiple operators in one project', {tags: ['e2e','admin']}, () => {
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
})
