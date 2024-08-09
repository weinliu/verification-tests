import { guidedTour } from '../../upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';
import { projectsPage } from '../../views/projects';
import { searchPage } from 'views/search';

let login_user_one:any, login_passwd_one:any, login_user_two:any, login_passwd_two:any;

describe('project list tests', () => {
  before(() => {
    const up_pair = Cypress.env('LOGIN_USERS').split(',');
    const [a, b] = up_pair;
    login_user_one = a.split(':')[0];
    login_passwd_one = a.split(':')[1];
    login_user_two = b.split(':')[0];
    login_passwd_two = b.split(':')[1];
    cy.uiLogin(Cypress.env('LOGIN_IDP'), login_user_one, login_passwd_one);
    guidedTour.close();
    cy.createProject('testuserone-project');
    cy.uiLogout();

    cy.uiLogin(Cypress.env('LOGIN_IDP'), login_user_two, login_passwd_two);
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject('testusertwo-project');
    cy.adminCLI(`oc adm policy add-role-to-user admin ${login_user_two} -n testuserone-project`);
  });

  after(() => {
    cy.adminCLI('oc delete project testuserone-project');
    cy.adminCLI('oc delete project testusertwo-project');
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user_two}`);
  });

  it('(OCP-43131,yapei,UserInterface) normal and admin user able to filter projects with Requester',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.log('normal user able to filter with Requester');
    cy.visit('/k8s/cluster/projects');
    listPage.rows.shouldBeLoaded();
    listPage.filter.byName('testuser');
    projectsPage.checkProjectExists("testuserone-project");
    projectsPage.checkProjectExists("testusertwo-project");
    // filter by Me
    projectsPage.filterMyProjects();
    projectsPage.checkProjectExists("testusertwo-project");
    projectsPage.checkProjectNotExists("testuserone-project");
    searchPage.clearAllFilters();

    // filter by User
    projectsPage.filterUserProjects();
    projectsPage.checkProjectExists("testuserone-project");
    projectsPage.checkProjectNotExists("testusertwo-project");
    searchPage.clearAllFilters();


    cy.log('cluster admin user able to filter with Requester');
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${login_user_two}`)
    cy.visit('/k8s/cluster/projects');
    // filter by System
    projectsPage.filterSystemProjects();
    projectsPage.checkProjectExists("openshift");
    listPage.filter.byName('testuser');
    projectsPage.checkProjectNotExists("testusertwo-project");
    searchPage.clearAllFilters();

    // filter by User
    listPage.filter.byName('testuser');
    projectsPage.filterUserProjects();
    projectsPage.checkProjectExists("testuserone-project");
    projectsPage.checkProjectNotExists("testusertwo-project");
    searchPage.clearAllFilters();

    // filter by Me
    listPage.filter.byName('testuser');
    projectsPage.filterMyProjects();
    projectsPage.checkProjectExists("testusertwo-project");
    projectsPage.checkProjectNotExists("testuserone-project");
  });
})
