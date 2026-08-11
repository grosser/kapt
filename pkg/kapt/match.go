package kapt

import (
	"fmt"
	"slices"
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// matcher is a compiled matchConstraints or binding matchResources, selectors are
// converted once so validating many resources stays cheap.
// scope and matchPolicy are ignored since they need a live apiserver to resolve.
type matcher struct {
	source            string // "matchConstraints" or "binding <name>", so skip reasons say who rejected
	rules             []admissionregistrationv1.NamedRuleWithOperations
	excludeRules      []admissionregistrationv1.NamedRuleWithOperations
	objectSelector    labels.Selector
	namespaceSelector labels.Selector
}

func newMatcher(source string, match *admissionregistrationv1.MatchResources) (*matcher, error) {
	everything := labels.Everything()
	if match == nil {
		return &matcher{source: source, objectSelector: everything, namespaceSelector: everything}, nil
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
		source:            source,
		rules:             match.ResourceRules,
		excludeRules:      match.ExcludeResourceRules,
		objectSelector:    objectSelector,
		namespaceSelector: namespaceSelector,
	}, nil
}

// match reports if the policy and at least one of its bindings select the resource,
// matchConditions are checked during validation.
func (p *Policy) match(resource *Resource) (bool, string) {
	if matched, reason := p.matcher.match(resource, p.namespaces); !matched {
		return false, reason
	}

	reasons := make([]string, 0, len(p.bindingMatchers))
	for _, binding := range p.bindingMatchers {
		matched, reason := binding.match(resource, p.namespaces)
		if matched {
			return true, ""
		}
		reasons = append(reasons, reason)
	}
	return false, strings.Join(reasons, ", ") // no binding selected it, so show why each did not
}

// namespaces is nil when no inventory was loaded, then namespaceSelector is ignored
func (m *matcher) match(resource *Resource, namespaces map[string]*corev1.Namespace) (bool, string) {
	if len(m.rules) > 0 && !matchesAnyRule(m.rules, resource) {
		return false, m.reason("resourceRules")
	}
	if matchesAnyRule(m.excludeRules, resource) {
		return false, m.reason("excludeResourceRules")
	}
	if !m.objectSelector.Matches(labels.Set(resource.GetLabels())) {
		return false, m.reason("objectSelector")
	}
	if namespaces == nil || m.namespaceSelector.Empty() {
		return true, ""
	}

	namespace, found := namespaces[resource.GetNamespace()]
	if !found {
		missing := fmt.Sprintf("namespaceSelector: namespace %s missing from inventory", resource.GetNamespace())
		return false, m.reason(missing)
	}
	if !m.namespaceSelector.Matches(labels.Set(namespace.Labels)) {
		return false, m.reason("namespaceSelector")
	}
	return true, ""
}

// reason names the field that rejected the resource and the matcher it belongs to
func (m *matcher) reason(field string) string {
	return m.source + " " + field
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
