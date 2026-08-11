package kapt

import (
	"fmt"
	"io"
	"os"
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
)

// Resource is a resource to validate, unstructured so we do not need to know its type.
type Resource = unstructured.Unstructured

// Policy is a compiled ValidatingAdmissionPolicy plus the bindings that select resources for it.
type Policy struct {
	Policy          *admissionregistrationv1.ValidatingAdmissionPolicy
	Bindings        []*admissionregistrationv1.ValidatingAdmissionPolicyBinding
	validator       validating.Validator
	matcher         *matcher
	bindingMatchers []*matcher
	namespaces      map[string]*corev1.Namespace // nil until LoadNamespaces was called
}

// LoadPolicy reads the ValidatingAdmissionPolicy and its bindings from a file with
// one or more yaml documents and compiles the policies CEL expressions.
func LoadPolicy(path string) (*Policy, error) {
	documents, err := loadDocuments(path)
	if err != nil {
		return nil, err
	}

	policy := &Policy{}
	found := 0
	for _, document := range documents {
		switch document.GetKind() {
		case "ValidatingAdmissionPolicy":
			found++
			policy.Policy = &admissionregistrationv1.ValidatingAdmissionPolicy{}
			if err = convert(document, policy.Policy); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		case "ValidatingAdmissionPolicyBinding":
			binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
			if err = convert(document, binding); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			policy.Bindings = append(policy.Bindings, binding)
		}
	}

	if found == 0 {
		return nil, fmt.Errorf("%s: found no ValidatingAdmissionPolicy", path)
	}
	if found > 1 {
		return nil, fmt.Errorf(
			"%s: found %d ValidatingAdmissionPolicy documents, kapt validates one policy per run\n"+
				"  split the file or loop: for p in policies/*/rule.yaml; do kapt $p resources.yaml; done",
			path, found,
		)
	}
	if policy.Policy.Spec.ParamKind != nil {
		return nil, fmt.Errorf("%s: paramKind is not supported", path)
	}
	policy.Bindings = keepBindingsFor(policy.Policy.Name, policy.Bindings)
	if len(policy.Bindings) == 0 {
		return nil, fmt.Errorf("%s: found no ValidatingAdmissionPolicyBinding for %s", path, policy.Policy.Name)
	}
	policy.validator = compilePolicy(policy.Policy)

	if policy.matcher, err = newMatcher("matchConstraints", policy.Policy.Spec.MatchConstraints); err != nil {
		return nil, fmt.Errorf("%s: matchConstraints: %w", path, err)
	}
	for _, binding := range policy.Bindings {
		bindingMatcher, err := newMatcher("binding "+binding.Name, binding.Spec.MatchResources)
		if err != nil {
			return nil, fmt.Errorf("%s: binding %s matchResources: %w", path, binding.Name, err)
		}
		policy.bindingMatchers = append(policy.bindingMatchers, bindingMatcher)
	}

	return policy, nil
}

// LoadResources reads all resources from the given files, "-" reads stdin.
// Lists (for example from `kubectl get deployments -A -o yaml`) are expanded into their items.
func LoadResources(paths ...string) ([]*Resource, error) {
	resources := []*Resource{}
	for _, path := range paths {
		documents, err := loadDocuments(path)
		if err != nil {
			return nil, err
		}
		for _, document := range documents {
			resources = append(resources, expandList(document)...)
		}
	}
	if len(resources) == 0 {
		return nil, fmt.Errorf("found no resources in %s", strings.Join(paths, " "))
	}
	return resources, nil
}

// LoadNamespaces reads the namespace inventory that namespaceSelector matching and the
// namespaceObject CEL variable need.
func (p *Policy) LoadNamespaces(path string) error {
	documents, err := loadDocuments(path)
	if err != nil {
		return err
	}

	p.namespaces = map[string]*corev1.Namespace{}
	for _, document := range documents {
		for _, item := range expandList(document) {
			if item.GetKind() != "Namespace" {
				return fmt.Errorf("%s: expected only Namespace resources, found %s", path, item.GetKind())
			}
			namespace := &corev1.Namespace{}
			if err = convert(item, namespace); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			p.namespaces[namespace.Name] = namespace
		}
	}
	return nil
}

// loadDocuments reads every non-empty yaml document from a file, "-" reads stdin.
func loadDocuments(path string) ([]*Resource, error) {
	reader, err := open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	documents := []*Resource{}
	decoder := utilyaml.NewYAMLOrJSONDecoder(reader, 4096)
	for {
		document := &Resource{}
		err = decoder.Decode(document)
		if err == io.EOF {
			return documents, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if document.GetKind() == "" {
			continue // skip empty / comment-only documents
		}
		documents = append(documents, document)
	}
}

func open(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

// expandList turns a List resource into its items and everything else into itself
func expandList(document *Resource) []*Resource {
	if !strings.HasSuffix(document.GetKind(), "List") {
		return []*Resource{document}
	}
	items := []*Resource{}
	_ = document.EachListItem(func(object runtime.Object) error {
		items = append(items, object.(*Resource))
		return nil
	})
	return items
}

// convert an unstructured document into a typed resource
func convert(document *Resource, into any) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(document.Object, into)
}

func keepBindingsFor(name string, bindings []*admissionregistrationv1.ValidatingAdmissionPolicyBinding) []*admissionregistrationv1.ValidatingAdmissionPolicyBinding {
	kept := []*admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
	for _, binding := range bindings {
		if binding.Spec.PolicyName == "" || binding.Spec.PolicyName == name {
			kept = append(kept, binding)
		}
	}
	return kept
}
