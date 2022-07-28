import * as yamlEditor from '../upstream/views/yaml-editor';
import { guidedTour } from '../upstream/views/guided-tour';
import { testName } from "../upstream/support";

describe("yaml editor tests", () => {
  before(() => {
    cy.login(
      Cypress.env("LOGIN_IDP"),
      Cypress.env("LOGIN_USERNAME"),
      Cypress.env("LOGIN_PASSWORD")
    );
    guidedTour.close();
    cy.createProject(testName);
  });

  after(() => {
    cy.exec(`oc delete project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  });

  it("(OCP-42019) Create multiple resources by importing yaml", () => {
    // import multiple resources
    // and check successful creation result on import yaml status page
    cy.byTestID('import-yaml').click();
    yamlEditor.isImportLoaded();
    cy.fixture('example-resources-1.yaml').then((resourcesYAML) => {
      yamlEditor.setEditorContent(resourcesYAML);
      yamlEditor.clickSaveCreateButton();
    });
    cy.contains('rror').as('failureMsg').should('exist');
    cy.byLegacyTestID('example-dc')
      .should('have.attr', 'href', `/k8s/ns/${testName}/deploymentconfigs/example-dc`);
    cy.byLegacyTestID(`${testName}`)
      .should('have.attr', 'href', `/k8s/cluster/namespaces/${testName}`);
    
    // retry failed, this time it will fail on yaml creation page
    cy.byTestID('retry-failed-resources').click().then(() => {
      yamlEditor.isImportLoaded();
      yamlEditor.clickSaveCreateButton();
    });
    cy.get('@failureMsg').should('exist');
    yamlEditor.clickCancelButton();
    
    // import more resources
    cy.byTestID('import-yaml').click();
    yamlEditor.isImportLoaded();
    cy.fixture('example-resources-2.yaml').then((resourcesYAML) => {
      yamlEditor.setEditorContent(resourcesYAML);
      yamlEditor.clickSaveCreateButton();
    });
    cy.contains('successfully created').should('exist');
    cy.byLegacyTestID('example-role')
      .should('have.attr', 'href', `/k8s/ns/${testName}/roles/example-role`);
    cy.byLegacyTestID('test-configmap')
      .should('have.attr', 'href', `/k8s/ns/${testName}/configmaps/test-configmap`);

    cy.byTestID('import-more-yaml').click();
    yamlEditor.isImportLoaded();
    cy.fixture('example-resources-3.yaml').then((resourcesYAML) => {
      yamlEditor.setEditorContent(resourcesYAML);
      yamlEditor.clickSaveCreateButton();
    });
    cy.contains('successfully created').should('exist');
  });
});