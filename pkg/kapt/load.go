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
	params          *Resource                    // nil unless the policy has a paramKind
	namespaces      map[string]*corev1.Namespace // nil until LoadNamespaces was called
}

// LoadPolicy reads the ValidatingAdmissionPolicy and its bindings from files with
// one or more yaml documents and compiles the policies CEL expressions.
// Multiple files are read as one, for policies that keep their bindings separate.
func LoadPolicy(paths ...string) (*Policy, error) {
	documents, err := loadDocuments(paths...)
	if err != nil {
		return nil, err
	}
	path := strings.Join(paths, " ") // name all files in error messages

	policy := &Policy{}
	found := 0
	others := []*Resource{} // documents that are neither policy nor binding, used for params
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
		default:
			others = append(others, document)
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
	policy.Bindings = keepBindingsFor(policy.Policy.Name, policy.Bindings)
	if len(policy.Bindings) == 0 {
		return nil, fmt.Errorf("%s: found no ValidatingAdmissionPolicyBinding for %s", path, policy.Policy.Name)
	}
	if policy.params, err = findParams(policy.Policy, policy.Bindings, others, path); err != nil {
		return nil, err
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

// loadDocuments reads every non-empty yaml document from the given files, "-" reads stdin.
func loadDocuments(paths ...string) ([]*Resource, error) {
	documents := []*Resource{}
	for _, path := range paths {
		found, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		documents = append(documents, found...)
	}
	return documents, nil
}

func loadFile(path string) ([]*Resource, error) {
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

// findParams resolves the single param document a policy with paramKind needs:
// it must be part of the policy file and every binding must reference it via paramRef name/namespace
func findParams(
	policy *admissionregistrationv1.ValidatingAdmissionPolicy,
	bindings []*admissionregistrationv1.ValidatingAdmissionPolicyBinding,
	others []*Resource,
	path string,
) (*Resource, error) {
	paramKind := policy.Spec.ParamKind
	if paramKind == nil {
		for _, binding := range bindings {
			if binding.Spec.ParamRef != nil {
				return nil, fmt.Errorf("%s: binding %s has paramRef but policy has no paramKind", path, binding.Name)
			}
		}
		if len(others) > 0 {
			return nil, fmt.Errorf("%s: found %s document but policy has no paramKind", path, others[0].GetKind())
		}
		return nil, nil
	}

	matches := []*Resource{}
	for _, document := range others {
		if document.GetAPIVersion() == paramKind.APIVersion && document.GetKind() == paramKind.Kind {
			matches = append(matches, document)
		} else {
			return nil, fmt.Errorf(
				"%s: found %s document but paramKind is %s/%s",
				path, document.GetKind(), paramKind.APIVersion, paramKind.Kind,
			)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf(
			"%s: found no %s %s document for paramKind, add it to the policy file",
			path, paramKind.APIVersion, paramKind.Kind,
		)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("%s: found %d %s documents, only one param is supported", path, len(matches), paramKind.Kind)
	}

	params := matches[0]
	for _, binding := range bindings {
		ref := binding.Spec.ParamRef
		if ref == nil {
			return nil, fmt.Errorf("%s: binding %s has no paramRef", path, binding.Name)
		}
		if ref.Selector != nil {
			return nil, fmt.Errorf("%s: binding %s paramRef selector is not supported, use name and namespace", path, binding.Name)
		}
		if ref.Name != params.GetName() || ref.Namespace != params.GetNamespace() {
			return nil, fmt.Errorf(
				"%s: binding %s paramRef %s/%s does not match param %s/%s",
				path, binding.Name, ref.Namespace, ref.Name, params.GetNamespace(), params.GetName(),
			)
		}
	}
	return params, nil
}
