package kapt

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	reset  = "\033[0m"
)

var colors = map[Verdict]string{Allowed: green, Denied: red, Skipped: yellow, Error: red}

// report prints all results as json or one line per resource
func report(results []Result, options Options, stdout io.Writer) {
	if options.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		check(encoder.Encode(results))
		return
	}

	for _, result := range results {
		line := fmt.Sprintf(
			"%s/%s %s/%s %s %s",
			result.APIVersion, result.Kind, result.Namespace, result.Name,
			colorize(strings.ToUpper(string(result.Verdict)), colors[result.Verdict], options.Color),
			result.Message,
		)
		fmt.Fprintln(stdout, strings.TrimRight(line, " "))
	}
}

func colorize(text string, color string, enabled bool) string {
	if !enabled {
		return text
	}
	return color + text + reset
}
