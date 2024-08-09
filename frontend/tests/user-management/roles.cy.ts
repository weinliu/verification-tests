import * as users from "../../views/users";
import { testName } from "../../upstream/support";
import { guidedTour } from '../../upstream/views/guided-tour';
import { listPage } from "../../upstream/views/list-page";

let login_user_one:any, login_passwd_one:any, login_user_two:any, login_passwd_two:any;
describe('Roles and RoleBindings tests', () => {
  before(() => {
    const up_pair = Cypress.env('LOGIN_USERS').split(',');
    const [a, b] = up_pair;
    login_user_one = a.split(':')[0];
    login_passwd_one = a.split(':')[1];
    login_user_two = b.split(':')[0];
    login_passwd_two = b.split(':')[1];
    // the first user is normal user
    cy.uiLogin(Cypress.env('LOGIN_IDP'), login_user_one, login_passwd_one);
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject(testName);
    cy.adminCLI(`oc create -f ./fixtures/test-group-and-clusterrolebinding.yaml`)
  });

  after(() => {
    cy.adminCLI(`oc delete project ${testName}`);
    cy.adminCLI(`oc delete -f ./fixtures/test-group-and-clusterrolebinding.yaml`);
    // the second user is cluster-admin user
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user_two}`);
  });

  it('(OCP-54744,yapei,UserInterface) Roles and RoleBindings basic function checks',{tags:['@userinterface','@e2e','@osd-ccs','@smoke','@level0']}, () => {
    const params = {
      'rolebinding_name': 'ns-rb-test',
      'rb_namespace': testName,
      'role_name': 'basic-user',
      'subject': 'User',
      'subject_name': login_user_one
    }
    // Normal user check default rolebindings on Projects -> RoleBindings tab
    const operationsAndChecks = () => {
      listPage.filter.by('namespace');
      cy.get('td').contains(`${login_user_one}`);
      cy.byLegacyTestID(`${testName}`).should('exist');
      listPage.filter.by('system');
      cy.byTestID('system:deployers').should('exist');
      cy.byTestID('system:image-builders').should('exist');
      cy.byTestID('system:image-pullers').should('exist');
    }
    cy.visit(`/k8s/cluster/projects/${testName}/roles`);
    listPage.rows.shouldBeLoaded();
    operationsAndChecks();

    // Normal user check default rolebindings on User Management -> Role Bindings -> project selected
    cy.visit(`/k8s/ns/${testName}/rbac.authorization.k8s.io~v1~RoleBinding`);
    listPage.rows.shouldBeLoaded();
    operationsAndChecks();

    // normal user create new namespace rolebinding
    listPage.clickCreateYAMLbutton();
    users.createRoleBinding.setRoleBindingName(params.rolebinding_name);
    users.createRoleBinding.selectRBNamespace(params.rb_namespace);
    users.createRoleBinding.selectRoleName(params.role_name);
    users.createRoleBinding.selectSubjectKind(params.subject);
    users.createRoleBinding.setSubjectName(params.subject_name);
    users.createRoleBinding.clickSubmitButton();

    // grant second user cluster-admin and login
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${login_user_two}`);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), login_user_two, login_passwd_two)

    // cluster admin check normal user rolebbindings on User Management -> Users -> xx -> RoleBindings
    cy.visit(`/k8s/cluster/user.openshift.io~v1~User/${login_user_one}/roles`)
    listPage.rows.shouldBeLoaded();
    cy.byTestID(params.rolebinding_name).should('exist');
    cy.byTestID(params.role_name).should('exist');

    // cluster admin user check Group RoleBindings
    cy.visit('/k8s/cluster/user.openshift.io~v1~Group/testgroup-OCP54744/roles');
    listPage.rows.shouldBeLoaded();
    listPage.filter.by('cluster');
    cy.byTestID('cluster-admin').should('exist');
    cy.byTestID('testRBgroup').should('exist');

    // cluster admin user checks system:image-builder RoleBindings
    cy.visit('/k8s/cluster/clusterroles/system:image-builder/bindings');
    listPage.rows.shouldBeLoaded();
    cy.byTestID('system:image-builders').should('exist');
  });
})
