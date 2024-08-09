import { quotaCard } from '../../views/overview';
import { quotaPage } from '../../views/quotas';
describe('quota related feature', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project test-ocp52470`);
    cy.adminCLI(`oc label namespace test-ocp52470 test=ocp52470`);
    cy.adminCLI(`oc apply -f fixtures/ocp-52470-cm.yaml -n test-ocp52470`);
    cy.adminCLI(`oc create resourcequota quota1 --hard=pods=4,requests.cpu=1,limits.cpu=2,limits.memory=1Gi,services=3,requests.nvidia.com/gpu=5,requests.storage=88,persistentvolumeclaims=10 -n test-ocp52470`);
    cy.adminCLI(`oc create resourcequota quota2 --hard=configmaps=2,openshift.io/imagestreams=8,secrets=7 -n test-ocp52470`);
    cy.adminCLI(`oc create clusterresourcequota testcrq1 --hard=services=10 --project-label-selector='test=ocp52470'`);
    cy.adminCLI(`oc create clusterresourcequota testcrq2 --hard=secrets=7,configmaps=2,pods=6,limits.memory=200Mi,requests.storage=50Gi --project-label-selector='test=ocp52470'`);

    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc delete resourcequota quota1 quota2 -n test-ocp52470`);
    cy.adminCLI(`oc delete project test-ocp52470`);
    cy.adminCLI(`oc delete clusterresourcequota testcrq1 testcrq2`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-52470,yanpzhan,UserInterface) Quota charts should support to show all resource types',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.visit('/k8s/cluster/projects/test-ocp52470');
    //check quota charts on overview quota card
    quotaCard.checkQuotaCollapsed('quota1');
    quotaCard.checkResourceQuotaInfo('quota1','8 resources','none are at quota');
    quotaCard.expandQuota('quota1');
    const quota1_resources = ['limits.cpu','limits.memory','persistentvolumeclaims','pods','requests.cpu','requests.nvidia.com/gpu','requests.storage','services'];
    quota1_resources.forEach(function (resourcename) {
      quotaCard.checkResourceChartListed('quota1',`${resourcename}`);
    });

    quotaCard.checkQuotaExpanded('quota2');
    quotaCard.checkResourceQuotaInfo('quota2','3 resources','1 resource reached quota');

    quotaCard.checkQuotaExpanded('testcrq1');
    quotaCard.checkResourceQuotaInfo('testcrq1','1 resource','none are at quota');

    quotaCard.checkQuotaCollapsed('testcrq2');
    quotaCard.checkResourceQuotaInfo('testcrq2','5 resources','1 resource reached quota');
    quotaCard.expandQuota('testcrq2');
    const testcrq2_resources = ['configmaps','limits.memory','pods','requests.storage','secrets'];
    testcrq2_resources.forEach(function (resourcename) {
      quotaCard.checkResourceChartListed('testcrq2',`${resourcename}`);
    });

    //check resources quota charts exist on quota page
    quotaPage.goToOneProjectQuotaPage('test-ocp52470','quota1');
    quota1_resources.forEach(function (resourcename) {
      quotaPage.checkResourceQuotaListed(`${resourcename}`);
    });
  });
})
