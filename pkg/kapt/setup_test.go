package kapt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const policyPath = "testdata/policy.yaml"
const resourcesPath = "testdata/resources.yaml"
const namespacesPath = "testdata/namespaces.yaml"

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "kapt")
}

// using an expectation would hide the backtrace if it goes wrong
func noError(err error) {
	if err != nil {
		panic(err)
	}
}

// run the cli with buffers instead of stdout/stderr, so output stays colorless and testable
func runKapt(argv ...string) (code int, stdout string, stderr string) {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}
	code = Run(argv, out, err)
	return code, out.String(), err.String()
}

// write content into a file in a temporary folder and pass its path to fn
func withFile(name string, content string, fn func(path string)) {
	folder, err := os.MkdirTemp("", "kapt")
	noError(err)
	defer func() { noError(os.RemoveAll(folder)) }()

	path := filepath.Join(folder, name)
	noError(os.WriteFile(path, []byte(content), 0600))
	fn(path)
}

// write a policy with the given spec and a resource file and pass both paths to fn
func withPolicy(spec string, resource string, fn func(policyPath string, resourcePath string)) {
	withFile("policy.yaml", policyWith(spec), func(policyPath string) {
		withFile("resource.yaml", resource, func(resourcePath string) {
			fn(policyPath, resourcePath)
		})
	})
}

func withStdin(content string, fn func()) {
	withFile("stdin.yaml", content, func(path string) {
		file, err := os.Open(path)
		noError(err)
		defer file.Close()

		old := os.Stdin
		os.Stdin = file
		defer func() { os.Stdin = old }()

		fn()
	})
}

// a binding for barePolicy that selects everything
const binding = `
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: test.example.com}
spec: {policyName: test.example.com, validationActions: [Deny]}
`

// a policy with the given spec and a binding that selects everything
func policyWith(spec string) string {
	return barePolicy(spec) + binding
}

// a policy with the given spec, name and failurePolicy are filled in
func barePolicy(spec string) string {
	return `
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata: {name: test.example.com}
spec:
  failurePolicy: Fail
` + spec
}

// a job that violates testdata/policy.yaml
func job(name string, namespace string) string {
	return "\napiVersion: batch/v1\nkind: Job\nmetadata: {name: " + name + ", namespace: " + namespace +
		"}\nspec: {backoffLimit: 100}\n"
}
