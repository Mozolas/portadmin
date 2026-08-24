// Command portadmin shows an interactive table of processes listening on TCP
// ports and lets you kill the one you no longer need.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/Mozolas/portadmin/internal/ui"
)

// version is set by the release build; for `go install` builds it is read from
// the module's build info instead.
var version = ""

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("portadmin", buildVersion())
			return
		default:
			fmt.Fprintf(os.Stderr, "portadmin: unknown argument %q (portadmin takes no options)\n", os.Args[1])
			os.Exit(2)
		}
	}

	if err := ui.Run(buildVersion()); err != nil {
		fmt.Fprintln(os.Stderr, "portadmin:", err)
		os.Exit(1)
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return normaliseVersion(info.Main.Version)
}

// normaliseVersion turns the module version into something worth showing: a
// real tag stays as it is, while "(devel)" and the pseudo-versions Go derives
// from an untagged commit become "dev".
func normaliseVersion(v string) string {
	if v == "" || v == "(devel)" || strings.HasPrefix(v, "v0.0.0-") {
		return "dev"
	}
	return v
}
