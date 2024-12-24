import { detailsPage } from "upstream/views/details-page";
import { Pages } from "views/pages";
import { storage } from "views/storage";
import { listPage } from "upstream/views/list-page";

describe('PVC tests', () => {
  const project_name = 'test-72032';
  const pvc_name = 'test-pvc';
  const volumesnapshot_name = pvc_name + '-snapshot';
  const deployment_name = 'example-deployment';
  const test_sc_name = 'test-sc';
  const test_sc_provisioner = 'no-provisioner';
  const login_idp = Cypress.env('LOGIN_IDP');
  const login_user = Cypress.env('LOGIN_USERNAME');
  const login_password = Cypress.env('LOGIN_PASSWORD');
  before(() => {
    cy.uiLogin(login_idp,login_user,login_password);
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user}`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete sc ${test_sc_name}`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete project ${project_name}`,{failOnNonZeroExit: false});
  });

  it('(OCP-72032,yapei,UserInterface)Cross storage class clone and restore',{tags:['@userinterface','@e2e','admin','@rosa','@osd-ccs','@hypershift-hosted']}, function() {
    cy.isEFSDeployed().then(result => {
      if (result === true) {
        this.skip();
      }
    });
    const query_pvc = `oc get pvc ${pvc_name} -n ${project_name} -o jsonpath={.status.phase}`;
    const query_snapshot = `oc get volumesnapshot ${pvc_name}-snapshot -n ${project_name} -o jsonpath={.status.readyToUse}`;
    // create PVC and make sure its Bound
    cy.adminCLI(`oc create -f ./fixtures/test-sc.yaml`);
    cy.adminCLI(`oc new-project ${project_name}`);
    cy.adminCLI(`oc adm policy add-role-to-user admin ${login_user} -n ${project_name}`);
    cy.adminCLI(`oc create -f ./fixtures/deployments.yaml -n ${project_name}`);
    Pages.gotoPVCCreationPage(project_name);
    storage.createPVC(`${pvc_name}`, '200', 'MiB');
    cy.adminCLI(`oc get pvc -n ${project_name}`)
      .its('stdout')
      .should('contain', `${pvc_name}`);
    cy.adminCLI(`oc set volume deployment/${deployment_name} --add --mount-path=/tmp -t persistentVolumeClaim --claim-name=${pvc_name}`)
      .its('stdout')
      .should('match', /deployment.*updated/);

    cy.checkCommandResult(query_pvc, 'Bound').then(() => {
      // Clone PVC can only choose storageclass from same provisioner
      Pages.gotoPVCDetailsPage(project_name, pvc_name);
      detailsPage.clickPageActionFromDropdown('Clone PVC');
      storage.checkSCDropdownText('clone').then($els => {
        const sc_list = Cypress._.map(Cypress.$.makeArray($els), 'innerText');
        expect(sc_list).not.to.include(test_sc_provisioner);
      })
    });

    // create VolumnSnapshot for further checks
    cy.byLegacyTestID('modal-cancel-action').click();
    Pages.gotoVolumeSnapshotListPage(project_name);
    listPage.clickCreateYAMLbutton();
    storage.createVolumnSnapshot(pvc_name);
    cy.checkCommandResult(query_snapshot, 'true', { retries: 6, interval: 10000 }).then(() => {
      // Restore as new PVC can only choose storageclass from same provisioner
      Pages.gotoVolumeSnapshotDetailPage(project_name,volumesnapshot_name);
      detailsPage.clickPageActionFromDropdown('Restore as new PVC');
      storage.checkSCDropdownText('restore').then($els => {
        const sc_list = Cypress._.map(Cypress.$.makeArray($els), 'innerText');
        expect(sc_list).not.to.include(test_sc_provisioner);
      })
    });
  })
})