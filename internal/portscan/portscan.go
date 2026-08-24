// Package portscan discovers processes that listen on TCP ports and enriches
// them with the information needed to tell one local dev server from another.
package portscan

import (
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Endpoint is a unique (pid, port) pair in the LISTEN state.
type Endpoint struct {
	PID  int32
	Port uint32
}

// Listener is a single row of the table: one process listening on one port.
type Listener struct {
	PID     int32
	Port    uint32
	Cwd     string
	Cmdline string
	Command string
	Project string
	Started time.Time
	Uptime  time.Duration
}

// Scan returns every process listening on a TCP port, sorted by port.
func Scan() ([]Listener, error) {
	conns, err := net.Connections("inet")
	if err != nil {
		return nil, err
	}

	endpoints := listeningEndpoints(conns)
	if len(endpoints) == 0 {
		return nil, nil
	}

	pids := make([]int32, 0, len(endpoints))
	seen := map[int32]bool{}
	for _, e := range endpoints {
		if !seen[e.PID] {
			seen[e.PID] = true
			pids = append(pids, e.PID)
		}
	}

	infos := collectProcesses(pids)

	listeners := make([]Listener, 0, len(endpoints))
	for _, e := range endpoints {
		info := infos[e.PID]
		l := Listener{
			PID:     e.PID,
			Port:    e.Port,
			Cwd:     info.cwd,
			Cmdline: strings.Join(info.cmdline, " "),
			Command: CommandLabel(info.name, info.cmdline),
			Project: ProjectName(info.cwd),
			Started: info.started,
		}
		if !info.started.IsZero() {
			l.Uptime = time.Since(info.started)
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// listeningEndpoints keeps only TCP sockets in the LISTEN state and collapses
// the IPv4/IPv6 duplicates that a single server usually produces.
func listeningEndpoints(conns []net.ConnectionStat) []Endpoint {
	unique := map[Endpoint]bool{}
	for _, c := range conns {
		if c.Status != "LISTEN" || c.Type != syscall.SOCK_STREAM {
			continue
		}
		if c.Pid <= 0 || c.Laddr.Port == 0 {
			continue
		}
		unique[Endpoint{PID: c.Pid, Port: c.Laddr.Port}] = true
	}

	out := make([]Endpoint, 0, len(unique))
	for e := range unique {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].PID < out[j].PID
	})
	return out
}

type procInfo struct {
	name    string
	cmdline []string
	cwd     string
	started time.Time
}

func collectProcesses(pids []int32) map[int32]procInfo {
	infos := make(map[int32]procInfo, len(pids))
	var missingCwd []int32

	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			infos[pid] = procInfo{}
			continue
		}

		var info procInfo
		if name, err := p.Name(); err == nil {
			info.name = name
		}
		if args, err := p.CmdlineSlice(); err == nil {
			info.cmdline = args
		}
		if cwd, err := p.Cwd(); err == nil {
			info.cwd = cwd
		}
		if ms, err := p.CreateTime(); err == nil && ms > 0 {
			info.started = time.UnixMilli(ms)
		}
		if info.cwd == "" {
			missingCwd = append(missingCwd, pid)
		}
		infos[pid] = info
	}

	// gopsutil cannot read the working directory on macOS, so fall back to a
	// single batched lsof call for the processes still missing one.
	if runtime.GOOS == "darwin" && len(missingCwd) > 0 {
		for pid, cwd := range lsofCwds(missingCwd) {
			info := infos[pid]
			info.cwd = cwd
			infos[pid] = info
		}
	}
	return infos
}
