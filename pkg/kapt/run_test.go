package kapt

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Run", func() {
	It("denies and allows resources", func() {
		code, stdout, stderr := runKapt(policyPath, resourcesPath)
		Expect(stdout).To(Equal(
			"batch/v1/Job apps/bad DENIED Do not set backoffLimit > 10 on bad\n" +
				"batch/v1/Job apps/good ALLOWED\n",
		))
		Expect(stderr).To(Equal("kapt: ignoring namespaceSelector, pass --inventory to honor it\n"))
		Expect(code).To(Equal(1))
	})

	It("exits 0 when nothing is denied", func() {
		withPolicy("  validations: [{expression: 'true'}]", job("good", "apps"), func(policy string, resource string) {
			code, stdout, stderr := runKapt(policy, resource)
			Expect(stdout).To(Equal("batch/v1/Job apps/good ALLOWED\n"))
			Expect(stderr).To(Equal(""))
			Expect(code).To(Equal(0))
		})
	})

	It("reads multiple files and stdin", func() {
		withStdin(job("piped", "apps"), func() {
			code, stdout, _ := runKapt("-inventory", namespacesPath, policyPath, resourcesPath, "-")
			Expect(stdout).To(ContainSubstring("apps/bad DENIED"))
			Expect(stdout).To(ContainSubstring("apps/piped DENIED"))
			Expect(code).To(Equal(1))
		})
	})

	It("prints json", func() {
		code, stdout, _ := runKapt("-json", "-inventory", namespacesPath, policyPath, resourcesPath)
		Expect(stdout).To(Equal(
			`[{"apiVersion":"batch/v1","kind":"Job","namespace":"apps","name":"bad","verdict":"denied",` +
				`"message":"Do not set backoffLimit > 10 on bad"},` +
				`{"apiVersion":"batch/v1","kind":"Job","namespace":"apps","name":"good","verdict":"allowed",` +
				`"message":""}]` + "\n",
		))
		Expect(code).To(Equal(1))
	})

	It("honors namespaceSelector when given an inventory", func() {
		withFile("job.yaml", job("bad", "disabled"), func(path string) {
			code, stdout, stderr := runKapt("-inventory", namespacesPath, policyPath, path)
			Expect(stdout).To(Equal("batch/v1/Job disabled/bad SKIPPED namespaceSelector\n"))
			Expect(stderr).To(Equal(""))
			Expect(code).To(Equal(0))
		})
	})

	It("exits 2 and prints usage without arguments", func() {
		code, stdout, stderr := runKapt()
		Expect(stdout).To(Equal(""))
		Expect(stderr).To(ContainSubstring("Usage: kapt"))
		Expect(stderr).To(ContainSubstring("-inventory string"))
		Expect(code).To(Equal(2))
	})

	It("exits 2 on unknown flag", func() {
		code, _, stderr := runKapt("-nope", policyPath, resourcesPath)
		Expect(stderr).To(ContainSubstring("flag provided but not defined"))
		Expect(code).To(Equal(2))
	})

	It("exits 2 when the policy cannot be read", func() {
		code, _, stderr := runKapt("testdata/missing.yaml", resourcesPath)
		Expect(stderr).To(ContainSubstring("kapt: open testdata/missing.yaml"))
		Expect(code).To(Equal(2))
	})

	It("exits 2 when the inventory cannot be read", func() {
		code, _, stderr := runKapt("-inventory", "testdata/missing.yaml", policyPath, resourcesPath)
		Expect(stderr).To(ContainSubstring("kapt: open testdata/missing.yaml"))
		Expect(code).To(Equal(2))
	})

	It("exits 2 when resources cannot be read", func() {
		code, _, stderr := runKapt(policyPath, "testdata/missing.yaml")
		Expect(stderr).To(ContainSubstring("kapt: open testdata/missing.yaml"))
		Expect(code).To(Equal(2))
	})

	It("exits 2 on evaluation error", func() {
		spec := "  validations: [{expression: 'object.spec.nope == 1'}]"
		withPolicy(spec, job("bad", "apps"), func(policy string, resource string) {
			code, stdout, _ := runKapt(policy, resource)
			Expect(stdout).To(ContainSubstring("ERROR expression 'object.spec.nope == 1' resulted in error"))
			Expect(code).To(Equal(2))
		})
	})

	It("sends user and groups", func() {
		spec := "  validations: [{expression: \"request.userInfo.username == 'me' && " +
			"request.userInfo.groups == ['a', 'b']\"}]"
		withPolicy(spec, job("bad", "apps"), func(policy string, resource string) {
			code, stdout, _ := runKapt("-user", "me", "-groups", "a,b,,", policy, resource)
			Expect(stdout).To(ContainSubstring("ALLOWED"))
			Expect(code).To(Equal(0))
		})
	})

	Describe("ValidateAll", func() {
		It("returns nothing for no resources", func() {
			policy, err := LoadPolicy(policyPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.ValidateAll(nil, Options{})).To(BeEmpty())
		})
	})

	Describe("isTerminal", func() {
		It("is false for a buffer", func() {
			Expect(isTerminal(nil)).To(BeFalse())
		})

		It("is false for a file", func() {
			withFile("out.txt", "", func(path string) {
				file, err := os.Open(path)
				noError(err)
				defer file.Close()
				Expect(isTerminal(file)).To(BeFalse())
			})
		})
	})
})
