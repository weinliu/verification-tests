import { Pages } from "views/pages";
import { MC } from "views/machine-config";

describe('MachineConfig related tests', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")}`);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-74602,yapei,UserInterface)Simplified view of MachineConfig configuration files)',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']},() => {
    const system_mc = '01-worker-kubelet';
    const system_mc_ssh = '99-worker-ssh';
    let mc_contents;
    cy.adminCLI(`oc get mc ${system_mc} -o jsonpath='{.spec.config.storage.files[0]}'`).then((result) => {
      mc_contents = JSON.parse(result.stdout);
      const { contents:{source}, mode, overwrite, path } = mc_contents;
      // check Configuration files details
      Pages.gotoMachineConfigDetailsPage(system_mc);
      MC.configurationFilesSection('exist');
      cy.log(`${path}${mode}${overwrite.toString()}${source}`);
      MC.checkConfigurationFileDetails(path, mode, overwrite.toString(), source);
    })
    // no Configuration files section when spec.storage.files null
    Pages.gotoMachineConfigDetailsPage(system_mc_ssh);
    MC.configurationFilesSection('not.exist');
  });
})