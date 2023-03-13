import { guidedTour } from 'upstream/views/guided-tour';
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
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
        guidedTour.close();
        cy.createProject(params.namespace);
        cy.adminCLI(`oc create -f ./fixtures/operators/${params.crdFileName}`);
        cy.adminCLI(`oc create -f ./fixtures/operators/${params.csvFileName} -n ${params.namespace}`);
    })
    after(() => {
        cy.adminCLI(`oc delete crd ${params.crdName}`);
        cy.adminCLI(`oc delete namespace ${params.namespace}`);
    })

    it('(OCP-29819,yapei) Dynamically Generate Create Operand Form', {tags: ['e2e','@osd-ccs']}, () => {
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
            expect(alldivs[0].textContent).to.have.string('Select')
            expect(alldivs[1].textContent).to.have.string('Password')
            expect(alldivs[2].textContent).to.have.string('Required Text')
            expect(alldivs[3].textContent).to.have.string('optionalRequiredText')

            // optional fields ordering
            expect(alldivs[4].textContent).to.have.string('Field Group')
            expect(alldivs[8].textContent).to.have.string('nestedFieldDependency')
            expect(alldivs[13].textContent).to.have.string('K8s Resource Prefix')
            expect(alldivs[14].textContent).to.have.string('Pod Count')
            expect(alldivs[15].textContent).to.have.string('Resource Requirements')
            expect(alldivs[17].textContent).to.have.string('Boolean Switch')
            expect(alldivs[18].textContent).to.have.string('Checkbox')
            expect(alldivs[19].textContent).to.have.string('Image Pull Policy')
            expect(alldivs[20].textContent).to.have.string('Update Strategy')
            expect(alldivs[21].textContent).to.have.string('Text')
            expect(alldivs[22].textContent).to.have.string('Number')
            expect(alldivs[23].textContent).to.have.string('Node Affinity')
            expect(alldivs[25].textContent).to.have.string('Pod Affinity')
            expect(alldivs[27].textContent).to.have.string('Pod Anti Affinity')
            expect(alldivs[29].textContent).to.have.string('Object With Array')
            expect(alldivs[39].textContent).to.have.string('Array With Object')
            expect(alldivs[41].textContent).to.have.string('Deeply Nested')
            expect(alldivs[51].textContent).to.have.string('arrayFieldDependency')
            expect(alldivs[53].textContent).to.have.string('fieldDependencyControl')
            expect(alldivs[54].textContent).to.have.string('[SCHEMA] Array Field Group')
            cy.get('button.pf-c-expandable-section__toggle')
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
        })
    })
})