import { guidedTour } from '../../upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';
import { projectsPage } from '../../views/projects';

let login_user_one:any, login_passwd_one:any, login_user_two:any, login_passwd_two:any;

describe('project list tests', () => {
    before(() => {
        const up_pair = Cypress.env('LOGIN_UP_PAIR').split(',');
        const [a, b] = up_pair;
        login_user_one = a.split(':')[0];
        login_passwd_one = a.split(':')[1];
        login_user_two = b.split(':')[0];
        login_passwd_two = b.split(':')[1];
        cy.login(Cypress.env('LOGIN_IDP'), login_user_one, login_passwd_one);
        guidedTour.close();
        cy.createProject('userone-project');
        cy.logout;

        cy.login(Cypress.env('LOGIN_IDP'), login_user_two, login_passwd_two);
        guidedTour.close();
        cy.createProject('usertwo-project');
        cy.exec(`oc adm policy add-role-to-user admin ${login_user_two} -n userone-project --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    });

    after(() => {
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user_two} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        cy.exec(`oc delete project userone-project --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc delete project usertwo-project --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout;
    });

    it('(OCP-43131, admin) normal user able to filter projects with Requester', () => {
        cy.visit('/k8s/cluster/projects');
        listPage.rows.shouldBeLoaded();
        projectsPage.checkProjectExists("userone-project");
        projectsPage.checkProjectExists("usertwo-project");
        // filter by Me
        projectsPage.filterMyProjects();
        projectsPage.checkProjectExists("usertwo-project");
        projectsPage.checkProjectNotExists("userone-project");
        listPage.filter.clearAllFilters();

        // filter by User
        projectsPage.filterUserProjects();
        projectsPage.checkProjectExists("userone-project");
        projectsPage.checkProjectNotExists("usertwo-project");
        listPage.filter.clearAllFilters();
    });

    it('(OCP-43131, admin) cluster admin user able to filter all projects with Requester', () => {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${login_user_two} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        cy.visit('/k8s/cluster/projects');
        // filter by System
        projectsPage.filterSystemProjects();
        projectsPage.checkProjectExists("openshift");
        projectsPage.checkProjectNotExists("usertwo-project");
        listPage.filter.clearAllFilters();

        // filter by User
        projectsPage.filterUserProjects();
        projectsPage.checkProjectExists("userone-project");
        projectsPage.checkProjectNotExists("usertwo-project");
        listPage.filter.clearAllFilters();

        // filter by Me
        projectsPage.filterMyProjects();
        projectsPage.checkProjectExists("usertwo-project");
        projectsPage.checkProjectNotExists("userone-project");
    });
})
