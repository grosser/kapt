package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/grosser/kapt/pkg/kapt"
)

var _ = Describe("kapt", func() {
	Describe("main", func() {
		It("validates", func() {
			code, stdout := runMain(
				"-inventory", "pkg/kapt/testdata/namespaces.yaml",
				"pkg/kapt/testdata/policy.yaml", "pkg/kapt/testdata/resources.yaml",
			)
			Expect(stdout).To(ContainSubstring("batch/v1/Job apps/bad DENIED"))
			Expect(code).To(Equal(1))
		})

		It("shows version", func() {
			code, stdout := runMain("version")
			Expect(stdout).To(Equal(kapt.Version + "\n"))
			Expect(code).To(Equal(0))
		})
	})
})
