package portscan

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// lsofCwds asks lsof for the working directory of the given PIDs in one call.
func lsofCwds(pids []int32) map[int32]string {
	if len(pids) == 0 {
		return nil
	}

	list := make([]string, 0, len(pids))
	for _, pid := range pids {
		list = append(list, strconv.Itoa(int(pid)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// -Fpn prints machine readable records: "p<pid>" followed by "n<path>".
	out, err := exec.CommandContext(ctx, "lsof", "-a", "-d", "cwd", "-Fpn", "-p", strings.Join(list, ",")).Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseLsofCwds(string(out))
}

func parseLsofCwds(out string) map[int32]string {
	cwds := map[int32]string{}
	var current int32

	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err != nil {
				current = 0
				continue
			}
			current = int32(pid)
		case 'n':
			if current != 0 {
				cwds[current] = strings.TrimSpace(line[1:])
			}
		}
	}
	return cwds
}
