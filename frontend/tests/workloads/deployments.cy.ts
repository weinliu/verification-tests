import { guidedTour } from "upstream/views/guided-tour";
import { Pages } from "views/pages";
import { searchPage } from 'views/search';
import { Deployment } from 'views/deployment';

describe('deployment page', () => {
  const params ={
    ns: 'test-66094',
    dcname: 'hook'
  }
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc delete project ${params.ns}`);
  });

  it('(OCP-66094,xiyuzhao,UserInterface) Deprecated DeploymentConfig to Deployments',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa','@hypershift-hosted']}, () => {
    /* Defined arry contains all the page that need to check the alert
       Note: The arguments should be passed when call the function within the loop
       */
    const pagesToCheck = [
      Pages.gotoCreateDeploymentconfigsFormView,
      Pages.gotoCreateDeploymentconfigsYamlView,
      Pages.gotoDeploymentConfigList,
      Pages.gotoDeploymentConfigDetailsTab,
      () => {
        Pages.gotoDeploymentConfigDetailsTab(params.ns,params.dcname)
      },
      () => {
        Pages.gotoSearch();
        searchPage.chooseResourceType('DeploymentConfig');
      },
      () => {
        cy.visit(`/topology/ns/${params.ns}?view=list`);
        cy.get('[aria-label="DeploymentConfig sub-resources"]').click();
      }
      ];

    cy.createProject(params.ns);
    cy.adminCLI(`oc apply -f ./fixtures/deploymentconfig.yaml -n ${params.ns}`);
    pagesToCheck.forEach(pageFunction => {
      pageFunction(params.ns);
      Deployment.checkAlert();
    });
  });
})
