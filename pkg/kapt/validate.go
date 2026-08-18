package kapt

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Verdict is the outcome of validating a single resource.
type Verdict string

const (
	Allowed Verdict = "allowed"
	Denied  Verdict = "denied"
	Skipped Verdict = "skipped" // the policy or one of its bindings does not select this resource
	Error   Verdict = "error"
)

// Result is the outcome of validating a single resource.
type Result struct {
	APIVersion string  `json:"apiVersion"`
	Kind       string  `json:"kind"`
	Namespace  string  `json:"namespace"`
	Name       string  `json:"name"`
	Verdict    Verdict `json:"verdict"`
	Message    string  `json:"message"` // deny message, reason for skipping or error
}

// Validate evaluates a single resource, simulating a CREATE request.
func (p *Policy) Validate(resource *Resource, options Options) Result {
	if resource.GetNamespace() == "" {
		// the apiserver defaults a missing namespace to "default" before admission
		resource = resource.DeepCopy()
		resource.SetNamespace("default")
	}

	result := Result{
		APIVersion: resource.GetAPIVersion(),
		Kind:       resource.GetKind(),
		Namespace:  resource.GetNamespace(),
		Name:       resource.GetName(),
	}

	if matched, reason := p.match(resource); !matched {
		result.Verdict = Skipped
		result.Message = reason
		return result
	}

	decisions := p.validator.Validate(
		context.Background(),
		resourceFor(resource),
		attributes(resource, options),
		nil, // versionedParams, paramKind is not supported
		p.namespaces[resource.GetNamespace()],
		celconfig.RuntimeCELCostBudget,
		nil, // authorizer
	).Decisions

	if len(decisions) == 0 {
		result.Verdict = Skipped
		result.Message = "matchConditions"
		return result
	}

	result.Verdict = Allowed
	for _, decision := range decisions {
		message := strings.TrimSpace(decision.Message)
		if decision.Evaluation == validating.EvalError {
			return Result{result.APIVersion, result.Kind, result.Namespace, result.Name, Error, message}
		}
		if decision.Action == validating.ActionDeny && result.Verdict != Denied {
			result.Verdict = Denied
			result.Message = message
		}
	}
	return result
}

func attributes(resource *Resource, options Options) *admission.VersionedAttributes {
	gvk := resource.GroupVersionKind()
	record := admission.NewAttributesRecord(
		resource, nil, gvk,
		resource.GetNamespace(), resource.GetName(), resourceFor(resource), "",
		admission.Create, nil, false,
		&user.DefaultInfo{Name: options.User, Groups: options.Groups},
	)
	return &admission.VersionedAttributes{Attributes: record, VersionedObject: resource, VersionedKind: gvk}
}

// resourceFor guesses the resource name of a resource, the same way client-go does when
// it has no discovery information (Job -> jobs, Ingress -> ingresses, Policy -> policies)
// but with a fix for kinds ending in a vowel + "y", which client-go gets wrong
// (AccessKey -> accesskeies instead of accesskeys)
func resourceFor(resource *Resource) schema.GroupVersionResource {
	gvk := resource.GroupVersionKind()
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	kind := strings.ToLower(gvk.Kind)
	if endsInVowelY := len(kind) > 1 && strings.HasSuffix(kind, "y") &&
		strings.ContainsRune("aeiou", rune(kind[len(kind)-2])); endsInVowelY {
		gvr.Resource = kind + "s"
	}
	return gvr
}
