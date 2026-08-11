package kapt

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apiserver/pkg/admission/plugin/cel"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/matchconditions"
	"k8s.io/apiserver/pkg/cel/environment"
)

// compilePolicy mirrors the unexported validating.compilePolicy, building a
// Validator from a policy's variables, matchConditions and validations using
// the same CEL environment the apiserver uses.
func compilePolicy(policy *admissionregistrationv1.ValidatingAdmissionPolicy) validating.Validator {
	optionalVars := cel.OptionalVariableDeclarations{HasAuthorizer: true, StrictCost: true}
	expressionOptionalVars := cel.OptionalVariableDeclarations{StrictCost: true}
	failurePolicy := policy.Spec.FailurePolicy

	environmentSet := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion(), true)
	compositionEnv, err := cel.NewCompositionEnv(cel.VariablesTypeName, environmentSet)
	check(err)
	compiler := cel.NewCompositedCompilerFromTemplate(compositionEnv)
	compiler.CompileAndStoreVariables(convertVariables(policy.Spec.Variables), optionalVars, environment.StoredExpressions)

	var matcher matchconditions.Matcher
	if conditions := policy.Spec.MatchConditions; len(conditions) > 0 {
		accessors := make([]cel.ExpressionAccessor, len(conditions))
		for i := range conditions {
			accessors[i] = (*matchconditions.MatchCondition)(&conditions[i])
		}
		matcher = matchconditions.NewMatcher(
			compiler.CompileCondition(accessors, optionalVars, environment.StoredExpressions),
			failurePolicy, "policy", "validate", policy.Name,
		)
	}

	return validating.NewValidator(
		compiler.CompileCondition(convertValidations(policy.Spec.Validations), optionalVars, environment.StoredExpressions),
		matcher,
		compiler.CompileCondition(convertAuditAnnotations(policy.Spec.AuditAnnotations), optionalVars, environment.StoredExpressions),
		compiler.CompileCondition(convertMessageExpressions(policy.Spec.Validations), expressionOptionalVars, environment.StoredExpressions),
		failurePolicy,
	)
}

func convertValidations(in []admissionregistrationv1.Validation) []cel.ExpressionAccessor {
	out := make([]cel.ExpressionAccessor, len(in))
	for i, validation := range in {
		out[i] = &validating.ValidationCondition{
			Expression: validation.Expression,
			Message:    validation.Message,
			Reason:     validation.Reason,
		}
	}
	return out
}

func convertMessageExpressions(in []admissionregistrationv1.Validation) []cel.ExpressionAccessor {
	out := make([]cel.ExpressionAccessor, len(in))
	for i, validation := range in {
		if validation.MessageExpression != "" {
			out[i] = &validating.MessageExpressionCondition{MessageExpression: validation.MessageExpression}
		}
	}
	return out
}

func convertAuditAnnotations(in []admissionregistrationv1.AuditAnnotation) []cel.ExpressionAccessor {
	out := make([]cel.ExpressionAccessor, len(in))
	for i, annotation := range in {
		out[i] = &validating.AuditAnnotationCondition{Key: annotation.Key, ValueExpression: annotation.ValueExpression}
	}
	return out
}

func convertVariables(in []admissionregistrationv1.Variable) []cel.NamedExpressionAccessor {
	out := make([]cel.NamedExpressionAccessor, len(in))
	for i, variable := range in {
		out[i] = &validating.Variable{Name: variable.Name, Expression: variable.Expression}
	}
	return out
}
