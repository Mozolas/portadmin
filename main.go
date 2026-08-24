// Command portadmin shows an interactive table of processes listening on TCP
// ports and lets you kill the one you no longer need.
package main

import (
	"fmt"
	"os"

	"github.com/Mozolas/portadmin/internal/ui"
)

func main() {
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "portadmin:", err)
		os.Exit(1)
	}
}
