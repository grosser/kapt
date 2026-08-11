// Package kapt evaluates a Kubernetes ValidatingAdmissionPolicy against resources
// using the apiserver's own CEL compilation and validation logic.
package kapt

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
)

const Version = "v0.1.0"

const usage = `Usage: kapt [options] <policy.yaml> <resource.yaml>...

Evaluates a ValidatingAdmissionPolicy against every resource in the given files
("-" reads stdin) and prints one line per resource:

  <apiVersion>/<kind> <namespace>/<name> ALLOWED|DENIED|SKIPPED|ERROR <message>

Exit status: 0 = nothing denied, 1 = at least one denied, 2 = error

Options:
`

// Options configures how resources are evaluated and reported.
type Options struct {
	User      string   // request.userInfo.username
	Groups    []string // request.userInfo.groups
	Inventory string   // file with Namespace resources, needed for namespaceSelector and namespaceObject
	JSON      bool
	Color     bool
}

// Run is the command line interface, it never panics and returns the exit code.
func Run(argv []string, stdout io.Writer, stderr io.Writer) int {
	set, options, groups, noColor := newFlagSet(stderr)
	if err := set.Parse(argv); err != nil {
		return 2 // flag package already printed the error and usage
	}
	options.Groups = splitWithoutEmpty(*groups, ',')
	options.Color = isTerminal(stdout) && !*noColor

	paths := set.Args()
	if len(paths) < 2 {
		set.Usage()
		return 2
	}

	code, err := run(*options, paths[0], paths[1:], stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "kapt: %v\n", err)
		return 2
	}
	return code
}

func run(options Options, policyPath string, resourcePaths []string, stdout, stderr io.Writer) (int, error) {
	policy, err := LoadPolicy(policyPath)
	if err != nil {
		return 0, err
	}

	if options.Inventory == "" {
		if policy.hasNamespaceSelector() {
			fmt.Fprintln(stderr, "kapt: ignoring namespaceSelector, pass --inventory to honor it")
		}
	} else if err = policy.LoadNamespaces(options.Inventory); err != nil {
		return 0, err
	}

	resources, err := LoadResources(resourcePaths...)
	if err != nil {
		return 0, err
	}

	results := policy.ValidateAll(resources, options)
	report(results, options, stdout)

	code := 0
	for _, result := range results {
		if result.Verdict == Error {
			return 2, nil
		}
		if result.Verdict == Denied {
			code = 1
		}
	}
	return code, nil
}

// ValidateAll evaluates resources in parallel, but keeps the order of the input.
func (p *Policy) ValidateAll(resources []*Resource, options Options) []Result {
	results := make([]Result, len(resources))
	queue := make(chan int)

	var waitGroup sync.WaitGroup
	for range min(runtime.NumCPU(), max(len(resources), 1)) {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for i := range queue {
				results[i] = p.Validate(resources[i], options)
			}
		}()
	}

	for i := range resources {
		queue <- i
	}
	close(queue)
	waitGroup.Wait()

	return results
}

func newFlagSet(stderr io.Writer) (set *flag.FlagSet, options *Options, groups *string, noColor *bool) {
	options = &Options{}
	set = flag.NewFlagSet("kapt", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.Usage = func() {
		fmt.Fprint(stderr, usage)
		set.PrintDefaults()
	}
	set.StringVar(&options.Inventory, "inventory", "",
		"file with Namespace resources (for namespaceSelector and namespaceObject)")
	set.BoolVar(&options.JSON, "json", false, "print results as a json array")
	set.StringVar(&options.User, "user", "kapt", "request.userInfo.username")
	groups = set.String("groups", "system:masters,system:authenticated", "comma separated request.userInfo.groups")
	noColor = set.Bool("no-color", false, "disable colored output")
	return set, options, groups, noColor
}

func splitWithoutEmpty(joined string, delimiter rune) []string {
	return strings.FieldsFunc(joined, func(c rune) bool { return c == delimiter })
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}
