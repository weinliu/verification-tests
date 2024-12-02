const currentDirectory = Cypress.config('fixturesFolder');
let awsTempDir = `${currentDirectory}/aws-creds`;
let gcpTempDir = `${currentDirectory}/gcp-creds`;

export const LokiUtils = {
    prepareResourcesForLokiStack: (nameSpace, secretName, lokiBucketName?) => {
      LokiUtils.getPlatform()
      cy.get<string>('@ST').then(storageType => {
        switch(storageType){
          case 's3':
            AWSCreds.getAWSRegion();
            cy.get<string>('@AWSRegion').then(region => {
              //Create s3 bucket
              cy.exec(`aws s3api create-bucket --bucket ${lokiBucketName} --region ${region} --create-bucket-configuration LocationConstraint=${region}`, {failOnNonZeroExit: false})
              //Create s3 secret
              let endpoint = `https://s3.${region}.amazonaws.com`;
              AWSCreds.getAwsKeys();
              cy.get<string>('@AWSAccessKey').then(accessKey => {
                cy.get<string>('@AWSSecretKey').then(secretKey => {
                  cy.exec(`oc -n ${nameSpace} create secret generic ${secretName} \
                  --from-literal=endpoint="${endpoint}" \
                  --from-literal=region="${region}" \
                  --from-literal=bucketnames="${lokiBucketName}" \
                  --from-file=access_key_id=${accessKey} \
                  --from-file=access_key_secret=${secretKey} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
                })
                cy.exec(`rm -r ${awsTempDir}`)
              })
            })
            break;
          case 'azure':
            AzureCreds.getEnv().then((azure_env) => {
              AzureCreds.getStorageContainer().then((azure_storage_azure_container) => {
                AzureCreds.getStorageAccountName().then((azure_storage_azure_accountname) => {
                  AzureCreds.getStorageAccountKey().then((azure_storage_account_key) => {
                    AzureCreds.getEndpointSuffix().then((azure_endpoint_suffix) => {
                      cy.exec(`oc -n ${nameSpace} create secret generic ${secretName} \
                      --from-literal=environment="${azure_env}" \
                      --from-literal=container="${azure_storage_azure_container}" \
                      --from-literal=account_name="${azure_storage_azure_accountname}" \
                      --from-literal=account_key="${azure_storage_account_key}" \
                      --from-literal=endpoint_suffix="${azure_endpoint_suffix}" --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
                    })
                  })

                })
              })
            })
            break;
          case 'gcs':
            GCPCreds.getProjectID();
            cy.get<string>('@PROJECT_ID').then(projectID => {
              GCPCreds.createBucket(lokiBucketName, projectID);
            });
            GCPCreds.getServiceAccount();
            cy.get<string>('@GPCSA').then(serviceAccount => {
              cy.exec(`oc -n ${nameSpace} create secret generic ${secretName} \
              --from-literal=bucketname="${lokiBucketName}" \
              --from-file=key.json=${serviceAccount} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
            })
            cy.exec(`rm -r ${gcpTempDir}`);
            break;
          case 'swift':
            //TODO: CreateOpenStackContainer
            //TODO: createSecretForSwiftContainer
        }
      })
    },
    removeObjectStorage: (lokiBucketName?) => {
      LokiUtils.getPlatform()
      cy.get<string>('@ST').then(storageType => {
        switch(storageType){
          case 's3':
            //empty the bucket
            cy.exec(`aws s3 rm s3://${lokiBucketName} --recursive`, {failOnNonZeroExit: false})
            //remove the bucket
            cy.exec(`aws s3 rb s3://${lokiBucketName} --force`, {failOnNonZeroExit: false})
            break;
          case 'azure':
            break;
          case 'gcs':
            cy.exec(`gcloud storage rm --recursive gs://${lokiBucketName}`, {failOnNonZeroExit: false})
            break;
        }
      })
    },
    getPlatform: () => {
      cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get infrastructure cluster -o=jsonpath='{.status.platformStatus.type}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
      .then((result) => {
        switch (result.stdout) {
          case 'AWS': {
            cy.wrap('s3').as('ST')
            break;
          }
          case 'GCP': {
            cy.wrap('gcs').as('ST')
            break;
          }
          case 'Azure': {
            cy.wrap('azure').as('ST')
            break;
          }
          case 'Openstack': {
            cy.wrap('swift').as('ST')
            break;
          }
        }
      })
    },
    getStorageClass: () => {
      return cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get sc --no-headers | awk 'NR==1 {print $1}'`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return result.stdout;
        }
      })
    }
  };

  export const GCPCreds = {
    getProjectID: () => {
      cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get infrastructure cluster -o jsonpath='{.status.platformStatus.gcp.projectID}'`, {failOnNonZeroExit: false})
      .then((result) => {
        cy.wrap(`${result.stdout}`).as('PROJECT_ID');
      })
    },
    getServiceAccount: () => {
      cy.exec(`mkdir -p ${gcpTempDir}`)
      cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} extract secret/gcp-credentials -n kube-system --confirm --to=${gcpTempDir}`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          let path = `${gcpTempDir}/service_account.json`;
          cy.wrap(`${path}`).as('GPCSA');
        }
      })
    },
    createBucket: (bucketName, projectID) => {
      cy.exec(`gcloud storage buckets create gs://${bucketName} --project=${projectID}`, {failOnNonZeroExit: false})
    },
    listBucket: (bucketName) => {
      cy.exec(`gcloud storage ls --recursive gs://${bucketName}`, {failOnNonZeroExit: false})
    }
  };

  export const AWSCreds = {
    getAwsKeys: () => {
      cy.exec(`mkdir -p ${awsTempDir}`)
      cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} extract secret/aws-creds -n kube-system --confirm --to=${awsTempDir}`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          let accessKeyID = `${awsTempDir}/aws_access_key_id`;
          cy.wrap(`${accessKeyID}`).as('AWSAccessKey');
          let secretAccessKey = `${awsTempDir}/aws_secret_access_key`;
          cy.wrap(`${secretAccessKey}`).as('AWSSecretKey');
        }
      })
    },
    getAWSRegion: () => {
      cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get infrastructure cluster -o=jsonpath='{.status.platformStatus.aws.region}'`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          cy.wrap(`${result.stdout}`).as('AWSRegion');
        }
      })
    }
  };

  export const AzureCreds = {
    getStorageContainer: () => {
      return cy.exec(`oc  --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get deployment image-registry -o json -n openshift-image-registry | jq  -r '.spec.template.spec.containers[0].env[]|select(.name=="REGISTRY_STORAGE_AZURE_CONTAINER").value'`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return result.stdout;
        }
      })
    },
    getStorageAccountName: () => {
      return cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get deployment image-registry -o json -n openshift-image-registry| jq  -r '.spec.template.spec.containers[0].env[]|select(.name=="REGISTRY_STORAGE_AZURE_ACCOUNTNAME").value'`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return result.stdout;
        }
      })
    },
    getStorageAccountKey: () => {
      return cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get secret image-registry-private-configuration -o json -n openshift-image-registry |jq -r '.data.REGISTRY_STORAGE_AZURE_ACCOUNTKEY'|base64 -d`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return result.stdout;
        }
      })
    },
    getEndpointSuffix: () => {
      return cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get deployment image-registry -o json -n openshift-image-registry |jq  -r '.spec.template.spec.containers[0].env[]|select(.name=="REGISTRY_STORAGE_AZURE_REALM").value'`, {failOnNonZeroExit: false})
      .then((result) => {
        if(!result.stderr.includes('NotFound')) {
          return result.stdout;
        }
      })
    },
    getEnv: () => {
      return AzureCreds.getEndpointSuffix().then((AZURE_ENV) => {
        if(AZURE_ENV === "core.windows.net" || AZURE_ENV === "AzureUSGovernment") {
          return "AzureUSGovernment";
        } else {
          return "AzureGlobal";
        }
      })
    },
  };
