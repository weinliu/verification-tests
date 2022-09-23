import * as yamlEditor from '../upstream/views/yaml-editor'

export const importYamlPage = {
    dragDropYamlFile: (importfile: string) => {
      yamlEditor.clickSaveCreateButton();
      cy.get('.ocs-yaml-editor__root')
        .selectFile(importfile, {action: 'drag-drop'});
    }, 
    checkDangerAlert: (alertmsg: RegExp) => {
      cy.contains('h4', /error|Danger|alert/gi).should('exist');
      cy.contains('.pf-c-alert__description div', alertmsg).should('exist');
    }
  }