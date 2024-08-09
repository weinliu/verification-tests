import { Operand } from '../../views/operator-hub-page'

describe('operand form view', () => {
  const params = {
    'namespace': 'ocp29819-project',
    'csvFileName': 'ocp29819-csv.yaml',
    'csvName': 'mock-operator',
    'crdFileName': 'ocp29819-crd.yaml',
    'crdName': 'mock-resources.test.tectonic.com'
  }
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.createProject(params.namespace);
    cy.adminCLI(`oc create -f ./fixtures/operators/${params.crdFileName}`);
    cy.adminCLI(`oc create -f ./fixtures/operators/${params.csvFileName} -n ${params.namespace}`);
  })
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc delete crd ${params.crdName}`);
    cy.adminCLI(`oc delete namespace ${params.namespace}`);
  })

  it('(OCP-29819,yapei,UserInterface) Dynamically Generate Create Operand Form',{tags:['@userinterface','@e2e','admin','@osd-ccs']}, () => {
    cy.visit(`/k8s/ns/${params.namespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion/mock-operator/test.tectonic.com~v1~MockResource`)
    cy.byTestID('item-create').click()
    Operand.switchToFormView()
    // check form order
    cy.get('div[id^="root_spec_"]').then((alldivs) =>{
      // Form Order rules
      // 1. "required" fields with specDescriptors other than advanced
      // 2. "required" fields without specDescriptors
      // 3. "optional" fields with specDescriptors other than advanced
      // 4. "optional" fields without specDescriptors
      // 5. Fields with advanced specDescriptors wrapped in the 'Advanced Configuration' group

      // In CRD, we have required fields defined
      //  required:
      //    - password (has specDescriptors in CSV)
      //    - select (has specDescriptors in CSV)
      //    - requiredText (has specDescriptors in CSV)
      //    - optionalRequiredText (without specDescriptors in CSV)
      // in CSV specDescriptors, we have order: select -> password -> requiredText
      // CSV yaml matters? not CRD yaml
      // so the final order in form is: select -> password -> requiredText -> optionalRequiredText
      expect(alldivs[0].textContent).to.include('Select')
      expect(alldivs[1].textContent).to.include('Password')
      expect(alldivs[2].textContent).to.include('Required Text')
      expect(alldivs[3].textContent).to.include('optionalRequiredText')

      // optional fields ordering
      expect(alldivs[4].textContent).to.include('Field Group')
      expect(alldivs[8].textContent).to.include('nestedFieldDependency')
      expect(alldivs[13].textContent).to.include('K8s Resource Prefix')
      expect(alldivs[14].textContent).to.include('Pod Count')
      expect(alldivs[15].textContent).to.include('Resource Requirements')
      expect(alldivs[17].textContent).to.include('Boolean Switch')
      expect(alldivs[18].textContent).to.include('Checkbox')
      expect(alldivs[19].textContent).to.include('Image Pull Policy')
      expect(alldivs[20].textContent).to.include('Update Strategy')
      expect(alldivs[21].textContent).to.include('Text')
      expect(alldivs[22].textContent).to.include('Number')
      expect(alldivs[23].textContent).to.include('Node Affinity')
      expect(alldivs[25].textContent).to.include('Pod Affinity')
      expect(alldivs[27].textContent).to.include('Pod Anti Affinity')
      expect(alldivs[29].textContent).to.include('Object With Array')
      expect(alldivs[39].textContent).to.include('Array With Object')
      expect(alldivs[41].textContent).to.include('Deeply Nested')
      expect(alldivs[51].textContent).to.include('arrayFieldDependency')
      expect(alldivs[53].textContent).to.include('fieldDependencyControl')
      expect(alldivs[54].textContent).to.include('[SCHEMA] Array Field Group')
      cy.get('button')
        .contains('Advanced configuration')
        .scrollIntoView()
        .click();
      cy.get('#root_spec_advanced').should('exist');
      const texts = alldivs.map((i, el) => Cypress.$(el).text());
      const paragraphs = texts.get();
      // specDescriptor with "urn:alm:descriptor:com.tectonic.ui:hidden" will be hidden from the form
      expect(paragraphs).to.not.include('Hidden')
      // unresolved path (the path does not exist on the CRD schema) will NOT be displayed in the form
      expect(paragraphs).to.not.include('Namespace Selector')
      // unsupported type will not be shown on Operand details page
    })
    cy.get('#root_spec_requiredText').type('test required text');
    cy.get('#root_spec_optionalRequiredText').type('test required text without specDescriptor');
    Operand.submitCreation();
    cy.adminCLI(`oc get mockresource.test.tectonic.com -n ${params.namespace}`)
      .then((result) => {
        expect(result.stdout).contains("mock-resource-instance")
    })
    // unsupported specDescriptors will NOT be shown on Operand details
    cy.visit(`/k8s/ns/${params.namespace}/clusterserviceversions/mock-operator/test.tectonic.com~v1~MockResource/mock-resource-instance`);
    cy.contains('Filed Group').should('not.exist');
  })
})