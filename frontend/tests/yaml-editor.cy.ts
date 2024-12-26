import * as yamlEditor from '../upstream/views/yaml-editor';
import { guidedTour } from '../upstream/views/guided-tour';
import { testName } from "../upstream/support";
import { importYamlPage, yamlOptions } from "../views/yaml-page"
import { Pages } from 'views/pages';

describe("yaml editor tests", () => {
  before(() => {
    cy.cliLogin();
    cy.exec(`oc new-project ${testName}`);
    cy.login(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
    guidedTour.close();
  });

  after(() => {
    cy.exec(`oc delete project ${testName}`);
    cy.cliLogout();
  });

  it("(OCP-63312,yanpzhan,UserInterface) Add ability to show/hide tooltips in the yaml editor",{tags:['@userinterface','@e2e','@osd-ccs','@rosa']}, () => {
    cy.exec(`oc create -f ./fixtures/pods/example-pod.yaml -n ${testName}`);
    cy.visit(`/k8s/cluster/projects/${testName}/yaml`);
    yamlEditor.isLoaded();
    yamlOptions.setTooltips('show');
    yamlOptions.checkTooltipsVisibility('apiVersion', 'APIVersion defines the versioned schema', 'shown');
    cy.visit(`/k8s/ns/${testName}/pods/examplepod/yaml`);
    yamlEditor.isLoaded();
    yamlOptions.checkTooltipsVisibility('apiVersion', 'APIVersion defines the versioned schema', 'shown');
    yamlOptions.setTooltips('hide');
    yamlOptions.checkTooltipsVisibility('apiVersion', 'APIVersion defines the versioned schema', 'hidden');
    yamlOptions.setTooltips('show');
  });

  it("(OCP-21956,xiyuzhao,UserInterface) drag and drop file for Import YAML page",{tags:['@userinterface','@e2e','@osd-ccs','@rosa','@hypershift-hosted']}, () => {
    cy.visit(`/k8s/ns/${testName}/import`)
      .contains('[data-test-id="resource-title"]', "Import YAML");
    importYamlPage.dragDropYamlFile("./fixtures/fakelargefile.yaml");
    importYamlPage.checkDangerAlert(/exceed/gi);

    cy.fixture('default_operatorgroup.yaml').then((resourcesYAML) => {
      yamlEditor.setEditorContent(resourcesYAML);
      cy.byTestID('save-changes').click({force: true});
      importYamlPage.checkDangerAlert(/forbid/gi);
    });
  });

  it("(OCP-42019,yapei,UserInterface) Create multiple resources by importing yaml",{tags:['@userinterface','@e2e','@osd-ccs', '@smoke','@hypershift-hosted']},() => {
    // import multiple resources
    // and check successful creation result on import yaml status page
    cy.visit(`/k8s/cluster/projects/${testName}/yaml`);
    importYamlPage.open();
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
    // commenting out due to bug OCPBUGS-18286(affects >= 4.14)
/*     cy.byTestID('retry-failed-resources').click({force:true}).then(() => {
      yamlEditor.isImportLoaded();
      yamlEditor.clickSaveCreateButton();
    });
    cy.get('@failureMsg').should('exist');
    yamlEditor.clickCancelButton(); */

    // import more resources
    cy.visit(`/k8s/cluster/projects/${testName}/yaml`);
    importYamlPage.open();
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

  it("(OCP-68746,xiyuzhao,UserInterface) Yaml editor can handle a line of data longer than 78 characters",{tags:['@userinterface','@e2e','@osd-ccs','@rosa','@hypershift-hosted']}, () => {
    cy.exec(`oc create -f ./fixtures/configmap_with_multiple_characters.yaml -n ${testName}`);
    Pages.gotoConfigMapDetailsYamlTab(testName, "test-68746");
    cy.contains('span', 'eeee')
      .parents('.view-line')
      .next('.view-line')
      .should(($span) => {
        const text = $span.text().trim();
        expect(text).not.to.be.empty;
      })
  });
});
