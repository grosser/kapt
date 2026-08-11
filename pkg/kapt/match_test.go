package kapt

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// match a job in namespace apps against a policy with the given spec, using the namespace inventory
func matchJob(spec string, resource string) Result {
	var result Result
	withPolicy("  validations: [{expression: 'false', message: denied}]\n"+spec, resource,
		func(policyPath string, resourcePath string) {
			policy, err := LoadPolicy(policyPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.LoadNamespaces(namespacesPath)).To(Succeed())
			resources, err := LoadResources(resourcePath)
			Expect(err).ToNot(HaveOccurred())
			result = policy.Validate(resources[0], Options{})
		},
	)
	return result
}

var _ = Describe("match", func() {
	It("matches everything without matchConstraints", func() {
		Expect(matchJob("", job("bad", "apps")).Verdict).To(Equal(Denied))
	})

	It("matches wildcards", func() {
		spec := `  matchConstraints:
    resourceRules:
    - {apiGroups: ["*"], apiVersions: ["*"], operations: ["*"], resources: ["*"]}
`
		Expect(matchJob(spec, job("bad", "apps")).Verdict).To(Equal(Denied))
	})

	It("skips other resources", func() {
		spec := `  matchConstraints:
    resourceRules:
    - {apiGroups: ["apps"], apiVersions: ["v1"], operations: ["CREATE"], resources: ["deployments"]}
`
		result := matchJob(spec, job("bad", "apps"))
		Expect(result.Verdict).To(Equal(Skipped))
		Expect(result.Message).To(Equal("matchConstraints resourceRules"))
	})

	It("skips rules that do not include CREATE", func() {
		spec := `  matchConstraints:
    resourceRules:
    - {apiGroups: ["batch"], apiVersions: ["v1"], operations: ["DELETE"], resources: ["jobs"]}
`
		Expect(matchJob(spec, job("bad", "apps")).Message).To(Equal("matchConstraints resourceRules"))
	})

	It("matches resourceNames", func() {
		spec := `  matchConstraints:
    resourceRules:
    - {apiGroups: ["batch"], apiVersions: ["v1"], operations: ["CREATE"], resources: ["jobs"], resourceNames: ["bad"]}
`
		Expect(matchJob(spec, job("bad", "apps")).Verdict).To(Equal(Denied))
		Expect(matchJob(spec, job("other", "apps")).Message).To(Equal("matchConstraints resourceRules"))
	})

	It("skips excluded resources", func() {
		spec := `  matchConstraints:
    excludeResourceRules:
    - {apiGroups: ["batch"], apiVersions: ["*"], operations: ["*"], resources: ["jobs"]}
`
		Expect(matchJob(spec, job("bad", "apps")).Message).To(Equal("matchConstraints excludeResourceRules"))
	})

	It("skips resources not selected by objectSelector", func() {
		spec := "  matchConstraints: {objectSelector: {matchLabels: {keep: 'true'}}}\n"
		Expect(matchJob(spec, job("bad", "apps")).Message).To(Equal("matchConstraints objectSelector"))
	})

	It("skips resources in namespaces that are missing from the inventory", func() {
		spec := "  matchConstraints: {namespaceSelector: {matchLabels: {keep: 'true'}}}\n"
		Expect(matchJob(spec, job("bad", "elsewhere")).Message).To(
			Equal("matchConstraints namespaceSelector: namespace elsewhere missing from inventory"))
	})

	It("skips when all bindings do not match", func() {
		var result Result
		content := barePolicy("  validations: [{expression: 'false'}]") + `
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: a}
spec:
  policyName: test.example.com
  matchResources: {objectSelector: {matchLabels: {a: "true"}}}
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: b}
spec:
  policyName: test.example.com
  matchResources: {objectSelector: {matchLabels: {b: "true"}}}
`
		withFile("policy.yaml", content, func(policyPath string) {
			withFile("resource.yaml", job("bad", "apps"), func(resourcePath string) {
				policy, err := LoadPolicy(policyPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(policy.Bindings).To(HaveLen(2))
				resources, err := LoadResources(resourcePath)
				Expect(err).ToNot(HaveOccurred())
				result = policy.Validate(resources[0], Options{})
			})
		})
		Expect(result.Message).To(Equal("binding a objectSelector, binding b objectSelector"))
	})
})

var _ = Describe("hasNamespaceSelector", func() {
	It("is false without selectors", func() {
		withFile("policy.yaml", policyWith("  validations: [{expression: 'true'}]"), func(path string) {
			policy, err := LoadPolicy(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.hasNamespaceSelector()).To(BeFalse())
		})
	})

	It("is true when a binding has one", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(policy.hasNamespaceSelector()).To(BeTrue())
	})
})
