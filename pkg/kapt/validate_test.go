package kapt

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// validate a single resource against a policy with the given spec
func validate(spec string, resource string) Result {
	var result Result
	withPolicy(spec, resource, func(policyPath string, resourcePath string) {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		resources, err := LoadResources(resourcePath)
		Expect(err).ToNot(HaveOccurred())
		result = policy.Validate(resources[0], Options{User: "kapt"})
	})
	return result
}

var _ = Describe("Validate", func() {
	It("defaults a missing namespace to default, like the apiserver does", func() {
		spec := "  validations: [{expression: \"object.metadata.namespace == 'default'\"}]"
		result := validate(spec, "apiVersion: batch/v1\nkind: Job\nmetadata: {name: a}\n")
		Expect(result.Verdict).To(Equal(Allowed))
		Expect(result.Namespace).To(Equal("default"))
	})

	It("skips resources excluded by matchConditions", func() {
		spec := `  matchConditions: [{name: skip, expression: "object.metadata.name == 'other'"}]
  validations: [{expression: 'false'}]
`
		result := validate(spec, job("bad", "apps"))
		Expect(result.Verdict).To(Equal(Skipped))
		Expect(result.Message).To(Equal("matchConditions"))
	})

	It("prefers messageExpression over message, like the apiserver does", func() {
		spec := "  validations: [{expression: 'false', message: 'nope', messageExpression: \"'expression'\"}]"
		Expect(validate(spec, job("bad", "apps")).Message).To(Equal("expression"))
	})

	It("reports the first denial of multiple validations", func() {
		spec := "  validations: [{expression: 'false', message: first}, {expression: 'false', message: second}]"
		result := validate(spec, job("bad", "apps"))
		Expect(result.Verdict).To(Equal(Denied))
		Expect(result.Message).To(Equal("first"))
	})

	It("reports a compilation error", func() {
		result := validate("  validations: [{expression: 'nonsense(('}]", job("bad", "apps"))
		Expect(result.Verdict).To(Equal(Error))
		Expect(result.Message).To(ContainSubstring("compilation failed"))
	})

	It("uses audit annotations without breaking", func() {
		spec := `  validations: [{expression: 'true'}]
  auditAnnotations: [{key: a, valueExpression: "'b'"}]
`
		Expect(validate(spec, job("bad", "apps")).Verdict).To(Equal(Allowed))
	})

	It("exposes params from the policy file", func() {
		spec := "  validations: [{expression: \"object.spec.backoffLimit <= int(params.data.max)\", message: 'over the limit'}]"
		var result Result
		withFile("policy.yaml", paramPolicy(spec), func(policyPath string) {
			withFile("resource.yaml", job("bad", "apps"), func(resourcePath string) {
				policy, err := LoadPolicy(policyPath)
				Expect(err).ToNot(HaveOccurred())
				resources, err := LoadResources(resourcePath)
				Expect(err).ToNot(HaveOccurred())
				result = policy.Validate(resources[0], Options{})
			})
		})
		Expect(result.Verdict).To(Equal(Denied))
		Expect(result.Message).To(Equal("over the limit"))
	})

	It("can access the namespaceObject", func() {
		spec := "  validations: [{expression: \"namespaceObject.metadata.name == 'apps'\"}]"
		var result Result
		withPolicy(spec, job("bad", "apps"), func(policyPath string, resourcePath string) {
			policy, err := LoadPolicy(policyPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.LoadNamespaces(namespacesPath)).To(Succeed())
			resources, err := LoadResources(resourcePath)
			Expect(err).ToNot(HaveOccurred())
			result = policy.Validate(resources[0], Options{})
		})
		Expect(result.Verdict).To(Equal(Allowed))
	})
})

var _ = Describe("resourceFor", func() {
	It("guesses irregular plurals", func() {
		resources, err := LoadResources(resourcesPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourceFor(resources[0]).Resource).To(Equal("jobs"))

		resources[0].SetKind("Ingress")
		Expect(resourceFor(resources[0]).Resource).To(Equal("ingresses"))

		resources[0].SetKind("Policy")
		Expect(resourceFor(resources[0]).Resource).To(Equal("policies"))
	})

	It("does not turn vowel + y into ies", func() {
		resources, err := LoadResources(resourcesPath)
		Expect(err).ToNot(HaveOccurred())

		resources[0].SetKind("AccessKey")
		Expect(resourceFor(resources[0]).Resource).To(Equal("accesskeys"))

		resources[0].SetKind("UserSSHKey")
		Expect(resourceFor(resources[0]).Resource).To(Equal("usersshkeys"))
	})
})
