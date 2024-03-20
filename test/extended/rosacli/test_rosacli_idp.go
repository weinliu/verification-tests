package rosacli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service IDP/admin testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID string

		rosaClient *rosacli.Client
		idpService rosacli.IDPService

		rosaSensitiveClient *rosacli.Client
		idpServiceSensitive rosacli.IDPService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the clients")
		rosaClient = rosacli.NewClient()
		idpService = rosaClient.IDP
		rosaSensitiveClient = rosacli.NewSensitiveClient()
		idpServiceSensitive = rosaSensitiveClient.IDP
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		var errorList []error
		errorList = append(errorList, rosaClient.CleanResources(clusterID))
		errorList = append(errorList, rosaSensitiveClient.CleanResources(clusterID))
		o.Expect(errors.Join(errorList...)).ToNot(o.HaveOccurred())
	})

	g.It("Author:yuwan-Critical-35878-rosacli Create/describe/delete admin user by the rosacli command [Serial]", func() {
		var (
			idpType    = "htpasswd"
			idpName    = "myhtpasswd"
			usersValue = "testuser:asCHS-MSV5R-bUwmc-5qb9F"
		)

		g.By("Create admin")
		output, err := rosaSensitiveClient.User.CreateAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin account has been added"))

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(fmt.Sprintf("There is 'cluster-admin' user on cluster '%s'", clusterID)))

		g.By("List IDP")
		idpTab, _, err := idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeTrue())

		g.By("Delete admin")
		output, err = rosaClient.User.DeleteAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin user 'cluster-admin' has been deleted"))

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(fmt.Sprintf("There is no 'cluster-admin' user on cluster '%s'", clusterID)))

		g.By("List IDP after the admin is deleted")
		idpTab, _, err = idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeFalse())

		g.By("Create one htpasswd idp")
		output, err = idpService.CreateIDP(clusterID, idpName,
			"--type", idpType,
			"--users", usersValue,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idpName))

		g.By("Create admin")
		output, err = rosaSensitiveClient.User.CreateAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin account has been added"))
		commandOutput := rosaClient.Parser.TextData.Input(output).Parse().Output()
		command := strings.TrimLeft(commandOutput, " ")
		command = strings.TrimLeft(command, " ")
		command = regexp.MustCompile(`[\t\r\n]+`).ReplaceAllString(strings.TrimSpace(command), "\n")

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(fmt.Sprintf("There is 'cluster-admin' user on cluster '%s'", clusterID)))

		g.By("List IDP")
		idpTab, _, err = idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeTrue())
		o.Expect(idpTab.IsExist(idpName)).To(o.BeTrue())

		g.By("login the cluster with the created cluster admin")
		time.Sleep(3 * time.Minute)
		stdout, err := rosaSensitiveClient.Runner.RunCMD(strings.Split(command, " "))
		o.Expect(err).To(o.BeNil())
		o.Expect(stdout.String()).Should(o.ContainSubstring("Login successful"))
	})
	g.It("Author:mgahagan-Critical-35896-rosacli Create/List/Delete IDPs for rosa clusters by the rosa tool [Serial]", func() {
		// common IDP variables
		var (
			mappingMethod = "claim"
			clientID      = "cccc"
			clientSecret  = "ssss"
		)

		type theIDP struct {
			name string
			url  string // hostedDomain
			org  string
			// ldap
			bindDN            string
			bindPassword      string
			idAttribute       string
			usernameAttribute string
			nameAttribute     string
			emailAttribute    string
			// OpenID
			emailClaims   string
			nameClaims    string
			usernameClaim string
			extraScopes   string
		}

		idp := make(map[string]theIDP)
		idp["Github"] = theIDP{
			name: "mygithub",
			url:  "https://hn.com",
			org:  "myorg",
		}
		idp["LDAP"] = theIDP{
			name:              "myldap",
			url:               "ldap://myldap.com",
			bindDN:            "bddn",
			bindPassword:      "bdp",
			idAttribute:       "id",
			usernameAttribute: "usrna",
			nameAttribute:     "na",
			emailAttribute:    "ea",
		}
		idp["Google"] = theIDP{
			name: "mygoogle",
			url:  "google.com",
		}
		idp["Gitlab"] = theIDP{
			name: "mygitlab",
			url:  "https://gitlab.com",
		}
		idp["OpenID"] = theIDP{
			name:          "myopenid",
			url:           "https://google.com",
			emailClaims:   "ec",
			nameClaims:    "nc",
			usernameClaim: "usrnc",
			extraScopes:   "exts",
		}

		g.By("Create Github IDP")
		output, err := idpService.CreateIDP(clusterID, idp["Github"].name,
			"--mapping-method", mappingMethod,
			"--client-id", clientID,
			"--client-secret", clientSecret,
			"--hostname", idp["Github"].url,
			"--organizations", idp["Github"].org,
			"--type", "github")
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idp["Github"].name))

		g.By("Create Gitlab IDP")
		output, err = idpService.CreateIDP(clusterID, idp["Gitlab"].name,
			"--mapping-method", mappingMethod,
			"--client-id", clientID,
			"--client-secret", clientSecret,
			"--host-url", idp["Gitlab"].url,
			"--organizations", idp["Gitlab"].org,
			"--type", "gitlab")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idp["Gitlab"].name))

		g.By("Create Google IDP")
		output, err = idpService.CreateIDP(clusterID, idp["Google"].name,
			"--mapping-method", mappingMethod,
			"--client-id", clientID,
			"--client-secret", clientSecret,
			"--hosted-domain", idp["Google"].url,
			"--type", "google")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idp["Google"].name))

		g.By("Create LDAP IDP")
		output, err = idpService.CreateIDP(clusterID, idp["LDAP"].name,
			"--mapping-method", mappingMethod,
			"--bind-dn", idp["LDAP"].bindDN,
			"--bind-password", idp["LDAP"].bindPassword,
			"--url", idp["LDAP"].url,
			"--id-attributes", idp["LDAP"].idAttribute,
			"--username-attributes", idp["LDAP"].usernameAttribute,
			"--name-attributes", idp["LDAP"].nameAttribute,
			"--email-attributes", idp["LDAP"].emailAttribute,
			"--insecure",
			"--type", "ldap")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idp["LDAP"].name))

		g.By("Create OpenID IDP")
		output, err = idpService.CreateIDP(clusterID, idp["OpenID"].name,
			"--mapping-method", mappingMethod,
			"--client-id", clientID,
			"--client-secret", clientSecret,
			"--issuer-url", idp["OpenID"].url,
			"--username-claims", idp["OpenID"].usernameClaim,
			"--name-claims", idp["OpenID"].nameClaims,
			"--email-claims", idp["OpenID"].emailClaims,
			"--extra-scopes", idp["OpenID"].extraScopes,
			"--type", "openid")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idp["OpenID"].name))

		g.By("list all IDPs")
		idpTab, _, err := idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		for k := range idp {
			o.Expect(idpTab.IsExist(idp[k].name)).To(o.BeTrue(), "the idp %s is not in output", idp[k].name)
		}
	})

	g.It("Author:yuwan-Critical-49137-Create/Delete the HTPasswd IDPs by the rosacli command [Serial]", func() {
		var (
			idpType            = "htpasswd"
			idpNames           = []string{"htpasswdn1", "htpasswdn2", "htpasswdn3"}
			singleUserName     string
			singleUserPasswd   string
			multipleuserPasswd []string
		)

		g.By("Create admin")
		output, err := rosaSensitiveClient.User.CreateAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData := rosaSensitiveClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin account has been added"))

		g.By("Create one htpasswd idp with multiple users")
		_, singleUserName, singleUserPasswd, err = rosacli.GenerateHtpasswdPair("user1", "pass1")
		o.Expect(err).To(o.BeNil())
		output, err = idpServiceSensitive.CreateIDP(
			clusterID, idpNames[0],
			"--type", idpType,
			"--username", singleUserName,
			"--password", singleUserPasswd,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaSensitiveClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idpNames[0]))
		o.Expect(textData).Should(o.ContainSubstring("To log in to the console, open"))
		o.Expect(textData).Should(o.ContainSubstring("and click on '%s'", idpNames[0]))

		g.By("Create one htpasswd idp with single users")
		multipleuserPasswd, err = rosacli.GenerateMultipleHtpasswdPairs(2)
		o.Expect(err).To(o.BeNil())
		output, err = idpServiceSensitive.CreateIDP(
			clusterID, idpNames[1],
			"--type", idpType,
			"--users", strings.Join(multipleuserPasswd, ","),
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaSensitiveClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idpNames[1]))
		o.Expect(textData).Should(o.ContainSubstring("To log in to the console, open"))
		o.Expect(textData).Should(o.ContainSubstring("and click on '%s'", idpNames[1]))

		g.By("Create one htpasswd idp with multiple users from the file")
		multipleuserPasswd, err = rosacli.GenerateMultipleHtpasswdPairs(3)
		o.Expect(err).To(o.BeNil())
		location, err := rosacli.CreateTempFileWithPrefixAndContent("htpasswdfile", strings.Join(multipleuserPasswd, "\n"))
		o.Expect(err).To(o.BeNil())
		defer os.RemoveAll(location)
		output, err = idpServiceSensitive.CreateIDP(
			clusterID, idpNames[2],
			"--type", idpType,
			"--from-file", location,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaSensitiveClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idpNames[2]))
		o.Expect(textData).Should(o.ContainSubstring("To log in to the console, open"))
		o.Expect(textData).Should(o.ContainSubstring("and click on '%s'", idpNames[2]))

		g.By("List IDP")
		idpTab, _, err := idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeTrue())
		for _, v := range idpNames {
			o.Expect(idpTab.Idp(v).Type).To(o.Equal("HTPasswd"))
			o.Expect(idpTab.Idp(v).AuthURL).To(o.Equal(""))
		}
	})
})
