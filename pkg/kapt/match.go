package kapt

import (
	"fmt"
	"slices"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// matcher is a compiled matchConstraints or binding matchResources, selectors are
// converted once so validating many resources stays cheap.
// scope and matchPolicy are ignored since they need a live apiserver to resolve.
type matcher struct {
	rules             []admissionregistrationv1.NamedRuleWithOperations
	excludeRules      []admissionregistrationv1.NamedRuleWithOperations
	objectSelector    labels.Selector
	namespaceSelector labels.Selector
}

func newMatcher(match *admissionregistrationv1.MatchResources) (*matcher, error) {
	if match == nil {
		return &matcher{objectSelector: labels.Everything(), namespaceSelector: labels.Everything()}, nil
	}

	objectSelector, err := newSelector(match.ObjectSelector)
	if err != nil {
		return nil, err
	}
	namespaceSelector, err := newSelector(match.NamespaceSelector)
	if err != nil {
		return nil, err
	}
	return &matcher{
		rules:             match.ResourceRules,
		excludeRules:      match.ExcludeResourceRules,
		objectSelector:    objectSelector,
		namespaceSelector: namespaceSelector,
	}, nil
}

// match reports if the policy and at least one of its bindings select the resource,
// matchConditions are checked during validation.
func (p *Policy) match(resource *Resource) (matched bool, reason string) {
	if matched, reason = p.matcher.match(resource, p.namespaces); !matched {
		return false, reason
	}
	for _, binding := range p.bindingMatchers {
		if matched, reason = binding.match(resource, p.namespaces); matched {
			return true, ""
		}
	}
	return false, reason // reason of the last binding, they are usually equal
}

// namespaces is nil when no inventory was loaded, then namespaceSelector is ignored
func (m *matcher) match(resource *Resource, namespaces map[string]*corev1.Namespace) (bool, string) {
	if len(m.rules) > 0 && !matchesAnyRule(m.rules, resource) {
		return false, "resourceRules"
	}
	if matchesAnyRule(m.excludeRules, resource) {
		return false, "excludeResourceRules"
	}
	if !m.objectSelector.Matches(labels.Set(resource.GetLabels())) {
		return false, "objectSelector"
	}
	if namespaces == nil || m.namespaceSelector.Empty() {
		return true, ""
	}

	namespace, found := namespaces[resource.GetNamespace()]
	if !found {
		return false, fmt.Sprintf("namespace %s missing from inventory", resource.GetNamespace())
	}
	if !m.namespaceSelector.Matches(labels.Set(namespace.Labels)) {
		return false, "namespaceSelector"
	}
	return true, ""
}

func (p *Policy) hasNamespaceSelector() bool {
	matchers := append([]*matcher{p.matcher}, p.bindingMatchers...)
	return slices.ContainsFunc(matchers, func(m *matcher) bool { return !m.namespaceSelector.Empty() })
}

func matchesAnyRule(rules []admissionregistrationv1.NamedRuleWithOperations, resource *Resource) bool {
	return slices.ContainsFunc(rules, func(rule admissionregistrationv1.NamedRuleWithOperations) bool {
		return matchesRule(rule, resource)
	})
}

func matchesRule(rule admissionregistrationv1.NamedRuleWithOperations, resource *Resource) bool {
	gvk := resource.GroupVersionKind()
	matchesName := len(rule.ResourceNames) == 0 || slices.Contains(rule.ResourceNames, resource.GetName())
	return matchesName &&
		contains(rule.Operations, admissionregistrationv1.Create) &&
		contains(rule.APIGroups, gvk.Group) &&
		contains(rule.APIVersions, gvk.Version) &&
		contains(rule.Resources, resourceFor(resource).Resource)
}

// contains checks a rules values, which support "*" as a wildcard
func contains[T ~string](values []T, wanted T) bool {
	return slices.ContainsFunc(values, func(value T) bool { return value == "*" || value == wanted })
}

// newSelector converts a selector, a missing selector matches everything
func newSelector(selector *metav1.LabelSelector) (labels.Selector, error) {
	if selector == nil {
		return labels.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(selector)
}
