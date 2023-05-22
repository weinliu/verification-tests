import * as yamlEditor from '../upstream/views/yaml-editor';
import { guidedTour } from '../upstream/views/guided-tour';
import { testName } from "../upstream/support";
import { importYamlPage} from "../views/yaml-page"

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
    cy.adminCLI(`oc delete project ${testName}`);
  });

  it("(OCP-21956,xiyuzhao) drag and drop file for Import YAML page", {tags: ['e2e','@osd-ccs','@rosa']}, () => {
    cy.visit(`/k8s/ns/${testName}/import`)
      .contains('[data-test-id="resource-title"]', "Import YAML");
    importYamlPage.dragDropYamlFile("./fixtures/fakelargefile.yaml");
    importYamlPage.checkDangerAlert(/Maximum|size|exceeded|limit/gi);

    cy.fixture('default_operatorgroup.yaml').then((resourcesYAML) => {
      yamlEditor.setEditorContent(resourcesYAML);
      cy.byTestID('save-changes').click({force: true});
      importYamlPage.checkDangerAlert(/forbidden|cannot|create/gi);
    });
  });

  it("(OCP-42019,yapei) Create multiple resources by importing yaml",{tags: ['e2e','@osd-ccs']},() => {
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
