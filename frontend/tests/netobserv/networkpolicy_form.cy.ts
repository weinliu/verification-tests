import { nwpolicyPage, nwpolicyPageSelectors } from '../../views/nwpolicy-page';
import { OverviewSelectors } from '../../views/overview';
import { OCCreds, OCCli } from '../../views/cluster-cliops';
import { podsPageUtils } from '../../views/pods';
import testFixture from '../../fixtures/network_policy_form_test.json'
import { helperfuncs } from '../../views/utils'
import { VerifyPolicyForm } from '../../views/nwpolicy-utils'

const projects: string[] = ['test0', 'test1', 'test2']
const podLabels: string[] = ["test-pods", 'test-pods2']
const pod_label_key = 'name';
const ns_label_key = 'kubernetes.io/metadata.name'
const fixtureFile = 'network_policy_form_test'
const operatingProject = 'test0'

describe.skip('(OCP-41858, OCP-45303) Console Network Policies form tests', function () {
    before('any test', function () {
        let tmpFile = `/tmp/${helperfuncs.getRandomName()}`
        cy.writeFile(tmpFile, JSON.stringify(testFixture))

        let creds: OCCreds = { idp: Cypress.env('LOGIN_IDP'), user: Cypress.env('LOGIN_USERNAME'), password: Cypress.env('LOGIN_PASSWORD'), kubeconfig: Cypress.env('KUBECONFIG_PATH') }
        cy.login(creds.idp, creds.user, creds.password);
        helperfuncs.clickIfExist(OverviewSelectors.skipTour)
        cy.switchPerspective('Administrator');
        this.cli = new OCCli(creds)
        this.creds = creds

        // create projects and deploy pods.
        for (let i = 0; i < projects.length; i++) {
            cy.createProject(projects[i])
            this.cli.createPods(tmpFile, projects[i])
        }

        cy.fixture(fixtureFile).as('testData')
        cy.exec(`rm ${tmpFile}`);

        /* set pod names aliases */
        projects.forEach((project) => {
            podLabels.forEach(label => {
                let modLabel = label.replace('-', '')
                let aliasPrefix = `${project}_${modLabel}`
                podsPageUtils.setProjectPodNamesAlias(project, label, aliasPrefix)
            })
        })

    })

    beforeEach('test', function () {
        cy.visit(nwpolicyPage.getProjectPolicyURL(operatingProject))
    })

    after(function () {
        projects.forEach((project) => {
            cy.deleteProject(project);
        })
        cy.logout();
    })

    describe("UI validating tests", function () {

        beforeEach('any UI validation tests', function () {
            if (this.currentTest.title.includes('kube:admin') && Cypress.env('LOGIN_IDP') != 'kube:admin') {
                this.skip()
            }
        })

        it('should validate network policy form (OCP-41858)', function () {
            nwpolicyPage.creatPolicyForm()

            cy.get('input[id="name"]').should('have.attr', 'name')
            cy.byTestID('Deny all ingress traffic__checkbox').should('not.be.checked')
            cy.byTestID('Deny all egress traffic__checkbox').should('not.be.checked')
            cy.get('button').contains('Add ingress rule').click()

            // verify ingress sources
            cy.byTestID(nwpolicyPageSelectors.addIngress).should('exist').click()
            nwpolicyPage.verifyIngressEgressOpts("Ingress")

            // verify egress destinations
            cy.byTestID(nwpolicyPageSelectors.addEgress).should('exist').click()
            nwpolicyPage.verifyIngressEgressOpts("Egress")

            cy.get('button').contains('Remove')

            // verify create and cancel button.
            cy.get(nwpolicyPageSelectors.savePolicy).should('exist')
            cy.get(nwpolicyPageSelectors.cancelButton).should('exist')
        })

        it('should show affected pods, (OCP-45303)', function () {
            nwpolicyPage.creatPolicyForm()

            const projectName = 'test0'
            const affectedPodsLinkText = 'show-affected-pods'

            // verify all pods in project show up when no labels are added.
            cy.visit(`k8s/ns/${projectName}/networkpolicies/~new/form`)
            cy.byTestID(affectedPodsLinkText).click()
            const verify = new VerifyPolicyForm(this.cli)
            verify.podslist(projectName)

            // verify tree toggle view
            cy.get(nwpolicyPageSelectors.podsTreeViewBtn).parents('.pf-c-tree-view__list').should('have.class', 'pf-c-tree-view__list')
            cy.get(nwpolicyPageSelectors.podsTreeViewBtn).click().click()

            // verify pods for label: name=test-pods2 in project test0
            cy.byTestID(nwpolicyPageSelectors.mainPodBtn).click()
            cy.byTestID(nwpolicyPageSelectors.labelName).type(pod_label_key)
            cy.byTestID(nwpolicyPageSelectors.labelValue).type(podLabels[1])
            cy.byTestID(affectedPodsLinkText).click()
            verify.podslist(projectName, `${pod_label_key}=${podLabels[1]}`)

            // verify message and no pods show up when there are none in project
            const newProject = 'test4'
            cy.createProject(newProject)
            cy.visit(`k8s/ns/${newProject}/networkpolicies/~new/form`)
            cy.byTestID(affectedPodsLinkText).click()
            cy.byTestID(nwpolicyPageSelectors.podsPreviewTitle).should('have.text', "No pods matching the provided labels in the current namespace")
            cy.deleteProject(newProject)

        })

        it('kube:admin - should show affected max pods count, (OCP-45303)', function () {
            /* 
            verify the popover lists max 10 pods
            if there are more 10 pods in tree view, then it has footer link 
            ensure footer link has attributes to open in new tab upon clicking.
            */
            nwpolicyPage.creatPolicyForm()

            const projectName = 'test0'
            cy.visit(`k8s/ns/${projectName}/networkpolicies/~new/form`)

            cy.byTestID(nwpolicyPageSelectors.addIngress).click()
            cy.get(nwpolicyPageSelectors.dropdownBtn).should('have.text', 'Add allowed source').click()
            cy.get(nwpolicyPageSelectors.srcDestOptions[1]).click()
            cy.byTestID(nwpolicyPageSelectors.showIngressPods).click()
            cy.byTestID(nwpolicyPageSelectors.podsPreviewTree).should('be.visible')
            cy.get(nwpolicyPageSelectors.podsList).should('have.length', 10)
            cy.byTestID(nwpolicyPageSelectors.podsPreviewTree).siblings('a').then(link => {
                expect(link).to.have.attr('href', '/k8s/all-namespaces/pods')
                expect(link).to.have.attr('target', '_blank')
                expect(link).to.have.attr('rel', 'noopener noreferrer')
                cy.request(link.prop('href')).its('status').should('eq', 200)
            })
        })
    })

    describe("network policy end-to-end tests (OCP-41858)", function () {
        before('any end-to-end test', function () {
            /* map labels to number replicas for pods to those labels */
            const labels_npods = new Map()
            this.testData.items.filter(item => (item.kind == 'ReplicationController')).forEach((item) => { labels_npods.set(item.spec.template.metadata.labels.name, item.spec.replicas) })

            /* set aliases for pod IP */
            for (let p = 0; p < projects.length - 1; p++) {
                let project = projects[p]
                podLabels.forEach(label => {
                    let modLabel = label.replace('-', '')
                    let aliasPrefix = `${project}_${modLabel}`
                    for (let np = 0; np < labels_npods.get(label); np++) {
                        let podNameAlias = `${aliasPrefix}_pod${np}Name`
                        podsPageUtils.setPodIPAlias(project, this[podNameAlias])
                    }
                })
            }
        })

        beforeEach('end-to-end test', function () {
            cy.visit(nwpolicyPage.getProjectPolicyURL(operatingProject))
        })

        describe("ingress policies tests", function () {
            beforeEach('ingress tests', function () {
                nwpolicyPage.creatPolicyForm()
                cy.get(nwpolicyPageSelectors.nwPolicyName).type(helperfuncs.getRandomName())

                if (this.currentTest.title.includes('deny all')) {
                    return
                }
                cy.get('button').contains('Add ingress rule').click()
                cy.get('button.pf-c-dropdown__toggle.pf-c-button.pf-m-secondary').should('have.text', 'Add allowed source').click()
            })

            it('ingress: should verify deny all', function () {
                /* verify ingress traffic from one pod to another pod is not allowed in same project (test0)  */
                cy.byTestID('Deny all ingress traffic__checkbox').should('not.be.checked').click()
                cy.get(nwpolicyPageSelectors.savePolicy).click()

                /* verify all incoming traffic is rejected to pod 2 from pod 1 in NS test0 */
                let mod_label1 = podLabels[0].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label1_pod1Name = this[`${project0_label1_prefix}_pod1Name`]
                let verify = new VerifyPolicyForm(this.cli)
                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label1_pod1Name}IP`], 'timed out', false)
            })

            it('ingress: should allow pods from the same NS', function () {
                /*  apply policy to all pods in namespace only allow traffic from pods with label name=test-pods2 */
                cy.get(nwpolicyPageSelectors.srcDestOptions[0]).click()
                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic from pods in the same namespace')

                cy.get(nwpolicyPageSelectors.addPod).eq(1).within(() => {
                    cy.byTestID(nwpolicyPageSelectors.peerPodBtn).click()
                    cy.get(nwpolicyPageSelectors.label).type(pod_label_key)
                    cy.get(nwpolicyPageSelectors.selector).type(podLabels[1])
                })

                cy.get(nwpolicyPageSelectors.savePolicy).click()
                let mod_label1 = podLabels[0].replace('-', '')
                let mod_label2 = podLabels[1].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label2_prefix = `${projects[0]}_${mod_label2}`
                let project1_label1_prefix = `${projects[1]}_${mod_label1}`

                let verify = new VerifyPolicyForm(this.cli)

                /* connection from NS test0 with label:name=test-pods  to other pod with same label in same NS - should fail */

                let project0_label1_pod1Name = this[`${project0_label1_prefix}_pod1Name`]
                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label1_pod1Name}IP`], 'timed out', false)

                /* connection from NS test0 from pod with label:name=test-pods2 to pod with label:name=test-pods in same NS - should pass */
                verify.connection(projects[0], this[`${project0_label2_prefix}_pod0Name`], this[`${project0_label1_pod1Name}IP`], "Hello OpenShift")

                /* connection from NS test0 with label:name=test-pods to pod with label:name=test-pods2 - should fail */
                let project0_label2_pod0Name = this[`${project0_label2_prefix}_pod0Name`]
                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label2_pod0Name}IP`], 'timed out', false)

                /* connection from NS test1 from pod with label:name=test-pods2 to pod with label:name=test-pods in NS test0 - should fail */
                verify.connection(projects[1], this[`${project1_label1_prefix}_pod1Name`], this[`${project0_label1_pod1Name}IP`], 'timed out', false)
            })

            it('ingress: should allow pods from different NS', function () {
                cy.get(nwpolicyPageSelectors.srcDestOptions[1]).click()

                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic from pods inside the cluster')

                cy.get(nwpolicyPageSelectors.addNamespace).parent().within(() => {

                    cy.get(nwpolicyPageSelectors.addNamespace).within(() => {
                        cy.byTestID(nwpolicyPageSelectors.addNSBtn).click()

                        cy.get(nwpolicyPageSelectors.label).type(ns_label_key)
                        cy.get(nwpolicyPageSelectors.selector).type(projects[1])
                    })

                    cy.get(nwpolicyPageSelectors.addPod).within(() => {
                        cy.byTestID(nwpolicyPageSelectors.peerPodBtn).click()
                        cy.get(nwpolicyPageSelectors.label).type(pod_label_key)
                        cy.get(nwpolicyPageSelectors.selector).type(podLabels[1])
                    })
                })

                cy.get(nwpolicyPageSelectors.savePolicy).click()
                let mod_label1 = podLabels[0].replace('-', '')
                let mod_label2 = podLabels[1].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label2_prefix = `${projects[0]}_${mod_label2}`
                let project1_label2_prefix = `${projects[1]}_${mod_label2}`
                let project2_label2_prefix = `${projects[2]}_${mod_label2}`

                let project0label1pod1Name = this[`${project0_label1_prefix}_pod1Name`]
                let verify = new VerifyPolicyForm(this.cli)

                /* connection from pods with l:name=test-pods2 from NS test1 to pods in NS test0 should succeed. */
                verify.connection(projects[1], this[`${project1_label2_prefix}_pod1Name`], this[`${project0label1pod1Name}IP`], "Hello OpenShift")

                /* connection from pods with l:name=test-pods2 from NS test2 to pods in NS test0 should fail. */
                verify.connection(projects[2], this[`${project2_label2_prefix}_pod0Name`], this[`${project0label1pod1Name}IP`], 'timed out', false)

                /* connection from pods with l:name=test-pods2 in same NS test0 should fail */
                verify.connection(projects[0], this[`${project0_label2_prefix}_pod0Name`], this[`${project0label1pod1Name}IP`], 'timed out', false)

            })

            it("ingress: should validate peers by IP block", function () {
                cy.get(nwpolicyPageSelectors.srcDestOptions[2]).click()
                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic from peers by IP block')

                let [mod_label1, mod_label2] = [podLabels[0].replace('-', ''), podLabels[1].replace('-', '')]

                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project1_label1_prefix = `${projects[1]}_${mod_label1}`
                let project2_label2_prefix = `${projects[2]}_${mod_label2}`

                let project0label1pod0Name = this[`${project0_label1_prefix}_pod0Name`]
                let project1label1pod0Name = this[`${project1_label1_prefix}_pod0Name`]
                let project2label2pod0Name = this[`${project2_label2_prefix}_pod0Name`]

                const project1label1pod0IP = this[`${project1label1pod0Name}IP`]
                cy.get(nwpolicyPageSelectors.cidrField).type(`${project1label1pod0IP}/32`)
                cy.byTestID(nwpolicyPageSelectors.addPort).click()
                cy.get(nwpolicyPageSelectors.portField).type('8080')

                cy.get(nwpolicyPageSelectors.savePolicy).click()

                let project0label1pod0IP = this[`${project0label1pod0Name}IP`]
                let verify = new VerifyPolicyForm(this.cli)

                /* connection to pod in NS: test0 from NS: test1 pod0 IP should pass on port 8080 */
                verify.connection(projects[1], project1label1pod0Name, project0label1pod0IP, "Hello OpenShift")

                /* connection to pod in NS: test0 from NS: test1 pod0 IP should fail on port 8888 */
                verify.connection(projects[1], project1label1pod0Name, project0label1pod0IP, "timed out", false, "8888")

                /* connection to pod in NS: test0 from NS: test2 pod0 IP should fail on port 8080 */
                verify.connection(projects[2], project2label2pod0Name, project0label1pod0IP, "timed out", false)
            })
        })

        describe('egress policies tests', function () {
            beforeEach('egress test', function () {
                nwpolicyPage.creatPolicyForm()
                cy.get(nwpolicyPageSelectors.nwPolicyName).type(helperfuncs.getRandomName())

                if (this.currentTest.title.includes('deny all')) {
                    return
                }

                cy.get('button').contains('Add egress rule').click()
                cy.get('button.pf-c-dropdown__toggle.pf-c-button.pf-m-secondary').should('have.text', 'Add allowed destination').click()
            })

            it('egress: should verify deny all', function () {
                cy.byTestID('Deny all egress traffic__checkbox').should('not.be.checked').click()
                cy.get(nwpolicyPageSelectors.savePolicy).click()

                let mod_label1 = podLabels[0].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label1_pod1Name = this[`${project0_label1_prefix}_pod1Name`]

                let verify = new VerifyPolicyForm(this.cli)

                // verify connection can't be made from pod1 to pod0 in same NS.
                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label1_pod1Name}IP`], 'timed out', false)
            })

            it('egress: should allow traffic to pods in the same namespace', function () {
                cy.get(nwpolicyPageSelectors.srcDestOptions[0]).click()
                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic to pods in the same namespace')

                cy.get(nwpolicyPageSelectors.addPod).eq(1).within(() => {
                    cy.byTestID(nwpolicyPageSelectors.peerPodBtn).click()
                    cy.get(nwpolicyPageSelectors.label).type(pod_label_key)
                    cy.get(nwpolicyPageSelectors.selector).type(podLabels[1])
                })

                cy.get(nwpolicyPageSelectors.savePolicy).click()
                let mod_label1 = podLabels[0].replace('-', '')
                let mod_label2 = podLabels[1].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label2_prefix = `${projects[0]}_${mod_label2}`
                let project1_label2_prefix = `${projects[1]}_${mod_label2}`

                let verify = new VerifyPolicyForm(this.cli)


                let project0_label1_pod1Name = this[`${project0_label1_prefix}_pod1Name`]
                let project0_label2_pod0Name = this[`${project0_label2_prefix}_pod0Name`]

                /* connection from NS test0 with label:name=test-pods  to other pod with same label in same NS - should fail */

                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label1_pod1Name}IP`], 'timed out', false)

                /* connection from NS test0 from pod with label:name=test-pods2 to other pod with same label and in same NS - should pass */
                verify.connection(projects[0], this[`${project0_label2_prefix}_pod1Name`], this[`${project0_label2_pod0Name}IP`], "Hello OpenShift")

                /* connection from NS test0 with label:name=test-pods2 to pod with label:name=test-pods - should fail */
                let project0_label1_pod0Name = this[`${project0_label1_prefix}_pod0Name`]
                verify.connection(projects[0], this[`${project0_label2_prefix}_pod0Name`], this[`${project0_label1_pod0Name}IP`], 'timed out', false)

                /* connection from NS test1 from pod with label:name=test-pods to pod with label:name=test-pods in NS test1 - should pass */
                verify.connection(projects[1], this[`${project1_label2_prefix}_pod1Name`], this[`${project0_label1_pod0Name}IP`], 'Hello OpenShift')

            })

            it('egress: should allow traffic to different NS', function () {
                cy.get(nwpolicyPageSelectors.srcDestOptions[1]).click()

                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic to pods inside the cluster')

                cy.get(nwpolicyPageSelectors.addNamespace).parent().within(() => {
                    nwpolicyPage.addPodOrNamespace(nwpolicyPageSelectors.addNamespace, ns_label_key, projects[1])

                    nwpolicyPage.addPodOrNamespace(nwpolicyPageSelectors.addPod, pod_label_key, podLabels[1])
                })

                cy.get(nwpolicyPageSelectors.savePolicy).click()
                let mod_label1 = podLabels[0].replace('-', '')
                let mod_label2 = podLabels[1].replace('-', '')
                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label2_prefix = `${projects[0]}_${mod_label2}`
                let project1_label1_prefix = `${projects[1]}_${mod_label1}`
                let project1_label2_prefix = `${projects[1]}_${mod_label2}`

                let project0_label2_pod1Name = this[`${project0_label2_prefix}_pod1Name`]
                let project1_label1_pod1Name = this[`${project1_label1_prefix}_pod1Name`]
                let project1_label2_pod1Name = this[`${project1_label2_prefix}_pod1Name`]

                let verify = new VerifyPolicyForm(this.cli)

                /* connection from NS test0 pods should only succeed to pods in NS test1 pods with label name=test-pods2.
                   connections to all other pods should fail.
                */
                verify.connection(projects[0], this[`${project0_label1_prefix}_pod1Name`], this[`${project1_label1_pod1Name}IP`], "timed out", false)

                verify.connection(projects[0], this[`${project0_label1_prefix}_pod1Name`], this[`${project1_label2_pod1Name}IP`], "Hello OpenShift")

                verify.connection(projects[0], this[`${project0_label1_prefix}_pod0Name`], this[`${project0_label2_pod1Name}IP`], 'timed out', false)
            })

            it('egress: should allow traffic to peers by CIDR', function () {
                cy.get(nwpolicyPageSelectors.srcDestOptions[2]).click()
                cy.get(nwpolicyPageSelectors.peerHeader).should('have.text', 'Allow traffic to peers by IP block')

                let [mod_label1, mod_label2] = [podLabels[0].replace('-', ''), podLabels[1].replace('-', '')]

                let project0_label1_prefix = `${projects[0]}_${mod_label1}`
                let project0_label2_prefix = `${projects[0]}_${mod_label2}`
                let project1_label1_prefix = `${projects[1]}_${mod_label1}`

                let project0_label1_pod0Name = this[`${project0_label1_prefix}_pod0Name`]
                let project0_label2_pod0Name = this[`${project0_label2_prefix}_pod0Name`]

                const project0_label1_pod0IP = this[`${project0_label1_pod0Name}IP`]

                cy.get(nwpolicyPageSelectors.cidrField).type(`${project0_label1_pod0IP}/32`)
                cy.byTestID(nwpolicyPageSelectors.addPort).click()
                cy.get(nwpolicyPageSelectors.portField).type('8080')

                cy.get(nwpolicyPageSelectors.savePolicy).click()


                let verify = new VerifyPolicyForm(this.cli)

                /* pods in same NS test0 can only reach to pod whose CIDR and port matches; no other pod can be reached in NS test0 by other pods in same NS */
                verify.connection(projects[0], this[`${project0_label2_prefix}_pod0Name`], project0_label1_pod0IP, "Hello OpenShift")

                verify.connection(projects[0], this[`${project0_label2_prefix}_pod0Name`], project0_label1_pod0IP, "timed out", false, '8888')

                verify.connection(projects[0], this[`${project0_label1_prefix}_pod1Name`], this[`${project0_label2_pod0Name}IP`], "timed out", false)

                /* however pods from different NS can reach to any pods of NS test0 */
                verify.connection(projects[1], this[`${project1_label1_prefix}_pod1Name`], this[`${project0_label2_pod0Name}IP`], "Hello OpenShift")
            })
        })


        afterEach(() => {
            nwpolicyPage.deleteAllPolicies(operatingProject)
        });
    })
})
