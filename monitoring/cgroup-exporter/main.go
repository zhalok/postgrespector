package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const cgroupRoot = "/host/sys/fs/cgroup"

var (
	systemdScopeRe = regexp.MustCompile(`^docker-([0-9a-f]{64})\.scope$`)
	cgroupfsIDRe   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type containerStats struct {
	ID          string
	Name        string
	CPUSeconds  float64
	MemoryBytes uint64
	IORead      uint64
	IOWrite     uint64
	PIDs        uint64
}

func findContainerCgroups() []string {
	var dirs []string
	_ = filepath.WalkDir(cgroupRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if systemdScopeRe.MatchString(base) || cgroupfsIDRe.MatchString(base) {
			dirs = append(dirs, path)
		}
		return nil
	})
	return dirs
}

func extractContainerID(cgroupPath string) string {
	base := filepath.Base(cgroupPath)
	if m := systemdScopeRe.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	return base
}

func readUint(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return v
}

func readCPUSeconds(dir string) float64 {
	f, err := os.Open(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "usage_usec" {
			usec, _ := strconv.ParseUint(fields[1], 10, 64)
			return float64(usec) / 1_000_000
		}
	}
	return 0
}

func readIO(dir string) (read, write uint64) {
	f, err := os.Open(filepath.Join(dir, "io.stat"))
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		for _, field := range fields[1:] {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				continue
			}
			v, _ := strconv.ParseUint(kv[1], 10, 64)
			switch kv[0] {
			case "rbytes":
				read += v
			case "wbytes":
				write += v
			}
		}
	}
	return read, write
}

type dockerContainer struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
}

func containerNames() map[string]string {
	names := map[string]string{}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get("http://unix/containers/json")
	if err != nil {
		return names
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return names
	}

	for _, c := range containers {
		name := c.ID
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		names[c.ID] = name
	}

	return names
}

func collectStats() []containerStats {
	names := containerNames()

	var stats []containerStats
	seen := map[string]bool{}

	for _, dir := range findContainerCgroups() {
		id := extractContainerID(dir)
		if seen[id] {
			continue
		}
		seen[id] = true

		ioRead, ioWrite := readIO(dir)
		name, known := names[id]
		if !known {
			// docker.sock reports full 64-char IDs; fall back to the raw cgroup id.
			name = id
		}

		stats = append(stats, containerStats{
			ID:          id,
			Name:        name,
			CPUSeconds:  readCPUSeconds(dir),
			MemoryBytes: readUint(filepath.Join(dir, "memory.current")),
			IORead:      ioRead,
			IOWrite:     ioWrite,
			PIDs:        readUint(filepath.Join(dir, "pids.current")),
		})
	}

	return stats
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	stats := collectStats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	writeMetric := func(name, help, typ string, value func(containerStats) string) {
		fmt.Fprintf(w, "# HELP %s %s\n", name, help)
		fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
		for _, s := range stats {
			fmt.Fprintf(w, "%s{container_id=%q,container_name=%q} %s\n", name, s.ID, s.Name, value(s))
		}
	}

	writeMetric("cgroup_cpu_usage_seconds_total", "Cumulative CPU time consumed by the container", "counter",
		func(s containerStats) string { return fmt.Sprintf("%f", s.CPUSeconds) })
	writeMetric("cgroup_memory_usage_bytes", "Current memory usage in bytes", "gauge",
		func(s containerStats) string { return fmt.Sprintf("%d", s.MemoryBytes) })
	writeMetric("cgroup_io_read_bytes_total", "Cumulative bytes read from block devices", "counter",
		func(s containerStats) string { return fmt.Sprintf("%d", s.IORead) })
	writeMetric("cgroup_io_write_bytes_total", "Cumulative bytes written to block devices", "counter",
		func(s containerStats) string { return fmt.Sprintf("%d", s.IOWrite) })
	writeMetric("cgroup_pids", "Current number of PIDs in the container", "gauge",
		func(s containerStats) string { return fmt.Sprintf("%d", s.PIDs) })
}

func main() {
	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("cgroup-exporter listening on :9100")
	if err := http.ListenAndServe(":9100", nil); err != nil {
		panic(err)
	}
}
