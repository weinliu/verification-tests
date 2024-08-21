import { guidedTour } from '../../upstream/views/guided-tour';

let projectName: any;
describe('ImageStream Manifest', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    projectName = 'test-ocp59762'
    cy.createProject(projectName);
    cy.adminCLI(`oc import-image multiarch:latest --from=quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324 --import-mode='PreserveOriginal' --reference-policy=local --confirm -n ${projectName}`)
    cy.adminCLI(`oc import-image legacy:latest --from=quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324  --reference-policy=local --confirm -n ${projectName}`)
    cy.adminCLI(`oc import-image simple:latest --from=quay.io/openshifttest/ociimage@sha256:97923994fdc1c968eed6bdcb64be8e70d5356b88cfab0481cb6b73a4849361b7 --import-mode='PreserveOriginal' --reference-policy=local --confirm -n ${projectName}`)
  });

  after(() => {
    cy.adminCLI(`oc delete project ${projectName}`);
    cy.logout;
  });

  it('(OCP-59762,xiuwang,Image_Registry) Show manifest lists in the web console', {tags: ['e2e','@smoke']}, () => {
    cy.visit(`/k8s/ns/${projectName}/imagestreamtags/multiarch%3Alatest`)
    cy.get('[data-test-section-heading="ImageStreamTag details"]', {timeout: 60000}).should('be.visible');
    cy.contains('Supported Platforms').should('exist')
    cy.contains('amd').should('exist');
    cy.contains('arm').should('exist');
    cy.contains('arm64').should('exist');
    cy.contains('ppc64le').should('exist');
    cy.contains('riscv64').should('exist');
    cy.contains('s390x').should('exist');
    cy.visit(`/k8s/ns/${projectName}/imagestreamtags/legacy%3Alatest`)
    cy.contains('Supported Platforms').should('not.exist')
    cy.visit(`/k8s/ns/${projectName}/imagestreamtags/simple%3Alatest`)
    cy.contains('Supported Platforms').should('not.exist') 
  })
});
