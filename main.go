package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	processName    = flag.String("processName", "", "process to monitor connections of")
	shouldPrintFDs = flag.Bool("printFDs", false, "print file descriptors to stdout")

	knownNetworkFDs = map[string]struct{}{
		"ipv4": struct{}{},
		"ipv6": struct{}{},
		"tcp":  struct{}{},
	}
)

func main() {
	flag.Parse()

	if len(*processName) == 0 {
		fmt.Printf("must provide a process name")
		os.Exit(1)
	}

	// Identify the PIDs for the given process name.
	pids, err := currentPids(*processName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Identify the open FDs for the known PIDs.
	fds, err := fdsForPIDs(pids)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if *shouldPrintFDs {
		printFDs(fds)
	}
}

func currentPids(processName string) ([]string, error) {
	cmd := exec.Command("pgrep", processName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	pieces := strings.Split(string(output), "\n")
	var filtered []string
	for _, piece := range pieces {
		if len(piece) > 0 {
			filtered = append(filtered, strings.TrimSpace(piece))
		}
	}

	return filtered, nil
}

func fdsForPIDs(pids []string) ([]FDInfo, error) {
	cmd := exec.Command(
		"lsof",
		"-p", strings.Join(pids, ","),
		"-F", "ptn",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var info []FDInfo
	var currPID string
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "p") {
			// The field is the start of a new PIDs open files.
			currPID = strings.TrimPrefix(lines[i], "p")
			i++
		} else if len(strings.TrimSpace(lines[i])) == 0 {
			continue
		}

		info = append(info, FDInfo{
			PID:  currPID,
			Type: strings.TrimPrefix(lines[i+1], "t"),
			Name: strings.TrimPrefix(lines[i+2], "n"),

			// NOTE: this is a very poor approximation, but it's good enough
			// for now.
			IsListening: !strings.Contains(lines[i+2], "->"),
		})
		i += 2
	}

	return filterOutNonNetworkFDs(info), nil
}

type FDInfo struct {
	PID  string
	Type string
	// TODO(ttacon): Later on resolve known ports from /etc/services
	Name        string
	IsListening bool
}

func filterOutNonNetworkFDs(fds []FDInfo) []FDInfo {
	var filtered []FDInfo
	for _, fd := range fds {
		if _, ok := knownNetworkFDs[strings.ToLower(fd.Type)]; ok {
			filtered = append(filtered, fd)
		}
	}

	return filtered
}

func listeningFragment(isListening bool) string {
	if isListening {
		return "LISTENING"
	}
	return "NOT_LISTENING"
}

func printFDs(fds []FDInfo) {
	now := time.Now().Unix()
	for _, fd := range fds {
		fmt.Printf("%d %s %s %s %s\n",
			now, fd.PID, fd.Type, fd.Name, listeningFragment(fd.IsListening))
	}
}
