# kapt [![Test](https://github.com/grosser/kapt/actions/workflows/test.yml/badge.svg)](https://github.com/grosser/kapt/actions?query=branch%3Amaster) [![coverage](https://img.shields.io/badge/coverage-100%25-success.svg)](https://github.com/grosser/kapt)

**K**ubernetes **A**dmission **P**olicy **T**ester: validate resources against a local `ValidatingAdmissionPolicy` or `MutatingAdmissionPolicy` (TODO)

- 🎉 **Instant** feedback on policy changes, no cluster and no `envtest` needed
- 🚀 Uses the **apiservers own** CEL compilation and validation code, so results match production
- 💰 Validates **thousands** of resources per second, to see what a policy change would break

```bash
go install github.com/grosser/kapt@latest
kapt policy.yaml resources.yaml
```

```
batch/v1/Job apps/bad DENIED Do not set backoffLimit > 10 on bad
batch/v1/Job apps/good ALLOWED
apps/v1/Deployment apps/web SKIPPED matchConstraints resourceRules
```

Exit status: `0` = nothing denied, `1` = at least one denied, `2` = error (bad input or CEL error)

## Usage

```
Usage: kapt [options] <policy.yaml> <resources.yaml>...

Options:
  -groups string
    	comma separated request.userInfo.groups (default "system:masters,system:authenticated")
  -inventory string
    	file with Namespaces (for namespaceSelector and namespaceObject)
  -json
    	print results as a json array
  -user string
    	request.userInfo.username (default "kapt")
```

- The policy file must hold exactly one `ValidatingAdmissionPolicy` and its `ValidatingAdmissionPolicyBinding`s,
  since a policy without a binding is ignored by the apiserver.
  To test multiple policies, loop: `for p in policies/*/rule.yaml; do kapt $p resources.yaml; done`
- Resource files can hold multiple documents and `List`s, `-` reads stdin
- Resources the policy does not select are reported as `SKIPPED` with the field that rejected them,
  prefixed by `matchConstraints` or `binding <name>`, `matchConditions` has no prefix since it is policy level

  ```
  batch/v1/Job apps/dba SKIPPED binding job-backoff-limit objectSelector
  apps/v1/Deployment apps/web SKIPPED matchConstraints resourceRules
  ```
- When no binding selects a resource, every bindings reason is shown:
  `binding a namespaceSelector, binding b objectSelector`

### Validate everything in a cluster against a policy change

```bash
kubectl get jobs,cronjobs -A -o json > resources.json
kubectl get namespaces -o json > namespaces.json
kapt --inventory namespaces.json policy.yaml resources.json | grep -v ALLOWED
```

### Use from tests

Any language can shell out to it and use the exit status or `--json` output, for example ruby:

```ruby
_out, err, status = Open3.capture3("kapt", "policy.yaml", resource_file)
raise err if status.exitstatus == 2
allowed = status.exitstatus.zero?
```

Or use it as a go library, see [pkg/kapt](pkg/kapt) for `LoadPolicy`, `LoadResources` and `ValidateAll`.

## Notes

- Simulates a `CREATE` request, so `oldObject` is not set
- `paramKind` is not supported
- `namespaceSelector` and the `namespaceObject` variable need `--inventory`, without it selectors are ignored
- `matchPolicy` and `scope` are ignored since they need a live apiserver to resolve
- Resource names are guessed from the kind the same way `client-go` does (`Ingress` -> `ingresses`)
- A missing namespace defaults to `default`, like the apiserver does
- Colors are used when stdout is a terminal, disable with `--no-color`
- `kapt version` to see the current version

## TODO

- support a policy without a binding via an extra flag
- validate multiple policies in one run, parsing the resources only once
- support `MutatingAdmissionPolicy`
- support `UPDATE` requests with `oldObject`
- support `paramKind`

## Makefile setup to use a consistent version of kapt

```
.PHONY: test-policies
test-policies: kapt
	$(KAPT) policy.yaml resources.yaml

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
KAPT ?= $(LOCALBIN)/kapt
KAPT_VERSION ?= v0.1.0

.PHONY: kapt
kapt: $(LOCALBIN) # Download kapt (replace existing if incorrect version)
	@(test -f $(KAPT) && $(KAPT) version | grep "$(KAPT_VERSION)" >/dev/null) || \
	(rm -f $(KAPT) && echo "Installing $(KAPT) $(KAPT_VERSION)" && \
	GOBIN=$(LOCALBIN) go install github.com/grosser/kapt@$(KAPT_VERSION))
```

## Development

```bash
make # build and test with 100% coverage enforcement
```

## Release

- never release a major version unless absolutely necessary, since that requires a /v2 path
- make new version commit that changes version in readme "Makefile setup" + `pkg/kapt/run.go`
- push and tag the commit

## Author

[Michael Grosser](https://grosser.it)<br/>
michael@grosser.it<br/>
License: MIT
