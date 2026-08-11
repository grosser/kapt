package kapt

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadPolicy", func() {
	It("loads policy and matching bindings", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(policy.Policy.Name).To(Equal("job-backoff-limit.example.com"))
		Expect(policy.Bindings).To(HaveLen(1))
	})

	It("fails without a binding", func() {
		withFile("policy.yaml", barePolicy("  validations: [{expression: 'true'}]"), func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("found no ValidatingAdmissionPolicyBinding for test.example.com")))
		})
	})

	It("ignores bindings of other policies", func() {
		content := barePolicy("  validations: [{expression: 'true'}]") + `
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: other}
spec: {policyName: other.example.com}
`
		withFile("policy.yaml", content, func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("found no ValidatingAdmissionPolicyBinding")))
		})
	})

	It("fails without a policy", func() {
		withFile("policy.yaml", job("bad", "apps"), func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("found no ValidatingAdmissionPolicy")))
		})
	})

	It("fails with multiple policies", func() {
		content := policyWith("  validations: [{expression: 'true'}]") + "\n---" +
			policyWith("  validations: [{expression: 'false'}]")
		withFile("policy.yaml", content, func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring(
				"found 2 ValidatingAdmissionPolicy documents, kapt validates one policy per run\n" +
					"  split the file or loop: for p in policies/*/rule.yaml; do kapt $p resources.yaml; done",
			)))
		})
	})

	It("fails on paramKind", func() {
		spec := "  paramKind: {apiVersion: v1, kind: ConfigMap}\n  validations: [{expression: 'true'}]"
		withFile("policy.yaml", policyWith(spec), func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("paramKind is not supported")))
		})
	})

	It("fails on broken yaml", func() {
		withFile("policy.yaml", "kind: [", func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("policy.yaml: error converting YAML to JSON")))
		})
	})

	It("fails on a policy that is not a policy", func() {
		withFile("policy.yaml", "kind: ValidatingAdmissionPolicy\nspec: 1\n", func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(HaveOccurred())
		})
	})

	It("fails on a binding that is not a binding", func() {
		content := policyWith("  validations: [{expression: 'true'}]") +
			"\n---\nkind: ValidatingAdmissionPolicyBinding\nspec: 1\n"
		withFile("policy.yaml", content, func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(HaveOccurred())
		})
	})

	It("fails on a broken objectSelector", func() {
		spec := `  validations: [{expression: 'true'}]
  matchConstraints:
    objectSelector: {matchExpressions: [{key: a, operator: Nope}]}
`
		withFile("policy.yaml", policyWith(spec), func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("matchConstraints: ")))
		})
	})

	It("fails on a broken binding namespaceSelector", func() {
		content := policyWith("  validations: [{expression: 'true'}]") + `
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: broken}
spec:
  policyName: test.example.com
  matchResources:
    namespaceSelector: {matchExpressions: [{key: a, operator: Nope}]}
`
		withFile("policy.yaml", content, func(path string) {
			_, err := LoadPolicy(path)
			Expect(err).To(MatchError(ContainSubstring("binding broken matchResources: ")))
		})
	})
})

var _ = Describe("LoadResources", func() {
	It("loads multiple documents from multiple files, ignoring comments", func() {
		resources, err := LoadResources(resourcesPath, resourcesPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(resources).To(HaveLen(4))
		Expect(resources[0].GetName()).To(Equal("bad"))
	})

	It("expands lists", func() {
		content := `
apiVersion: v1
kind: List
items:
- {apiVersion: batch/v1, kind: Job, metadata: {name: a}}
- {apiVersion: batch/v1, kind: Job, metadata: {name: b}}
`
		withFile("resources.yaml", content, func(path string) {
			resources, err := LoadResources(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))
			Expect(resources[1].GetName()).To(Equal("b"))
		})
	})

	It("mixes plain resources and multiple lists", func() {
		content := job("plain", "apps") + `
---
apiVersion: v1
kind: List
items:
- {apiVersion: batch/v1, kind: Job, metadata: {name: a}}
---
apiVersion: v1
kind: JobList
items:
- {apiVersion: batch/v1, kind: Job, metadata: {name: b}}
---
apiVersion: v1
kind: List
items: []
` + job("plain-2", "apps")
		withFile("resources.yaml", content, func(path string) {
			resources, err := LoadResources(path)
			Expect(err).ToNot(HaveOccurred())
			names := []string{}
			for _, resource := range resources {
				names = append(names, resource.GetName())
			}
			Expect(names).To(Equal([]string{"plain", "a", "b", "plain-2"}))
		})
	})

	It("fails without resources", func() {
		withFile("resources.yaml", "# nothing here\n", func(path string) {
			_, err := LoadResources(path)
			Expect(err).To(MatchError(ContainSubstring("found no resources in ")))
		})
	})

	It("fails on a missing file", func() {
		_, err := LoadResources("testdata/missing.yaml")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("LoadNamespaces", func() {
	It("loads namespaces", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(policy.LoadNamespaces(namespacesPath)).To(Succeed())
		Expect(policy.namespaces).To(HaveKey("apps"))
		Expect(policy.namespaces["disabled"].Labels).To(HaveKeyWithValue("policies-disabled", "true"))
	})

	It("fails on other resources", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(policy.LoadNamespaces(resourcesPath)).To(MatchError(ContainSubstring("expected only Namespace resources, found Job")))
	})

	It("fails on a missing file", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(policy.LoadNamespaces("testdata/missing.yaml")).To(HaveOccurred())
	})

	It("fails on a namespace that is not a namespace", func() {
		policy, err := LoadPolicy(policyPath)
		Expect(err).ToNot(HaveOccurred())
		withFile("namespaces.yaml", "kind: Namespace\nmetadata: 1\n", func(path string) {
			Expect(policy.LoadNamespaces(path)).To(HaveOccurred())
		})
	})
})
