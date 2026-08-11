package kapt

import (
	"bytes"
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("report", func() {
	results := []Result{
		{APIVersion: "batch/v1", Kind: "Job", Namespace: "apps", Name: "bad", Verdict: Denied, Message: "nope"},
		{APIVersion: "v1", Kind: "Pod", Namespace: "apps", Name: "good", Verdict: Allowed},
	}

	It("colors verdicts", func() {
		out := &bytes.Buffer{}
		report(results, Options{Color: true}, out)
		Expect(out.String()).To(Equal(
			"batch/v1/Job apps/bad \033[31mDENIED\033[0m nope\n" +
				"v1/Pod apps/good \033[32mALLOWED\033[0m\n",
		))
	})

	It("does not escape html in json", func() {
		out := &bytes.Buffer{}
		report([]Result{{Message: "a > b"}}, Options{JSON: true}, out)
		Expect(out.String()).To(ContainSubstring(`"message":"a > b"`))
	})
})

var _ = Describe("check", func() {
	It("panics on error", func() {
		Expect(func() { check(errors.New("oops")) }).To(Panic())
	})
})
