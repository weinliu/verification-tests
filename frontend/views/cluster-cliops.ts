
export interface OCCreds {
    idp: string;
    user: string;
    password: string;
    kubeconfig: string;
}

export class OCCli {
    creds: OCCreds
    loggedin;
    networkprovider: string

    constructor(creds: OCCreds) {
        this.creds = creds
        cy.log(JSON.stringify(this.creds))
        this.loggedin = this.login()
    }

    login(): void {
        cy.exec(`oc login -u ${this.creds.user} -p ${this.creds.password}`).then(result => {
            cy.log("logging error" + result.stderr)
            cy.log("logging output" + result.stdout)
            expect(result.stderr).to.be.empty
        })
    }

    createPods(specFilePath, project): void {
        cy.exec(`oc create -f ${specFilePath} -n ${project} --kubeconfig=${this.creds.kubeconfig}`);
    }

    switchProject(project: string): void {
        cy.exec(`oc project ${project}  --kubeconfig=${this.creds.kubeconfig}`).then(result => {
            expect(result.stderr).to.be.empty
        })
    }

    runPodCmd(project: string, podName: string, cmd: string, exOut: string, exResult: boolean = true) {
        cy.exec(`oc rsh -n ${project} ${podName} ${cmd}  --kubeconfig=${this.creds.kubeconfig}`, { failOnNonZeroExit: exResult }).then(result => {
            if (exResult) {
                this.matchOutput(result.stdout, exOut)
            }
            else {
                this.matchOutput(result.stderr, exOut)
            }
        })
    }

    private matchOutput(text: string, match: string = null) {
        if (match) {
            cy.wrap(text).then(text => {
                expect(text).to.contain(match)
            })
        }
        else {
            cy.wrap(text).then(text => {
                expect(text).to.be.not.empty
            })
        }
    }

    create_project(name: string): void {
        cy.exec(`oc new-project ${name} --kubeconfig=${this.creds.kubeconfig}`).then(result => {
            cy.log("logging error" + result.stderr)
            cy.log("logging output" + result.stdout)
            expect(result.stderr).to.be.empty
        })
    }

    apply_manifest(manifest: Object): void {
        let cmd = `echo '${JSON.stringify(manifest)}' | oc create --kubeconfig=${this.creds.kubeconfig} -f -`
        cy.exec(cmd).then(result => {
            expect(result.stderr).to.be.empty
        })
    }

    wait_pod_ready(label: string, namespace: string): void {
        let cmd = `oc wait --timeout=180s --for=condition=ready pod -l ${label} -n ${namespace} --kubeconfig=${this.creds.kubeconfig}`
        cy.exec(cmd).then(result => {
            expect(result.stderr).to.be.empty
        })
    }

    delete_project(name: string): void {
        let cmd = `oc delete project ${name}  --kubeconfig=${this.creds.kubeconfig}`
        cy.exec(cmd).then(result => {
            expect(result.stderr).to.be.empty
        })
    }

    delete_resources(manifest: Object): void {
        let cmd = `echo '${JSON.stringify(manifest)}' | oc delete --kubeconfig=${this.creds.kubeconfig} -f - `
        cy.exec(cmd).then(result => {
            expect(result.stderr).to.be.empty
        })
    }
}
