package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pollInterval = 2 * time.Second

type resolution struct {
	W, H int
}

func (r resolution) pixels() int { return r.W * r.H }
func (r resolution) String() string {
	return fmt.Sprintf("%dx%d", r.W, r.H)
}

type output struct {
	Name        string
	Connected   bool
	Primary     bool
	Resolutions []resolution
}

var (
	outputRe = regexp.MustCompile(`^(\S+)\s+(connected|disconnected)\s*(primary)?\s*`)
	modeRe   = regexp.MustCompile(`^\s+(\d+)x(\d+)\s+`)
)

func parseXrandr() ([]output, error) {
	cmd := exec.Command("xrandr", "--query")
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xrandr --query: %w", err)
	}

	var outputs []output
	var cur *output

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()

		if m := outputRe.FindStringSubmatch(line); m != nil {
			outputs = append(outputs, output{
				Name:      m[1],
				Connected: m[2] == "connected",
				Primary:   m[3] == "primary",
			})
			cur = &outputs[len(outputs)-1]
			continue
		}

		if cur != nil {
			if m := modeRe.FindStringSubmatch(line); m != nil {
				w, _ := strconv.Atoi(m[1])
				h, _ := strconv.Atoi(m[2])
				cur.Resolutions = append(cur.Resolutions, resolution{w, h})
			}
		}
	}
	return outputs, nil
}

// bestCommonResolution finds the highest-pixel-count resolution shared by all
// the given outputs. Falls back to the best resolution of the new output.
func bestCommonResolution(outputs []output) resolution {
	if len(outputs) == 0 {
		return resolution{1920, 1080}
	}

	// Build set from first output's resolutions.
	common := make(map[resolution]bool)
	for _, r := range outputs[0].Resolutions {
		common[r] = true
	}

	// Intersect with each subsequent output.
	for _, o := range outputs[1:] {
		have := make(map[resolution]bool)
		for _, r := range o.Resolutions {
			have[r] = true
		}
		for r := range common {
			if !have[r] {
				delete(common, r)
			}
		}
	}

	var shared []resolution
	for r := range common {
		shared = append(shared, r)
	}

	if len(shared) == 0 {
		// No common resolution — pick the best of the last (newly connected) output.
		last := outputs[len(outputs)-1]
		if len(last.Resolutions) > 0 {
			return last.Resolutions[0]
		}
		return resolution{1920, 1080}
	}

	sort.Slice(shared, func(i, j int) bool {
		return shared[i].pixels() > shared[j].pixels()
	})
	return shared[0]
}

func mirror(primary output, externals []output, res resolution) error {
	args := []string{
		"--output", primary.Name,
		"--mode", res.String(),
		"--pos", "0x0",
		"--primary",
	}
	for _, ext := range externals {
		args = append(args,
			"--output", ext.Name,
			"--mode", res.String(),
			"--same-as", primary.Name,
		)
	}

	log.Printf("xrandr %s", strings.Join(args, " "))
	cmd := exec.Command("xrandr", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func connectedSet(outputs []output) map[string]bool {
	s := make(map[string]bool)
	for _, o := range outputs {
		if o.Connected {
			s[o.Name] = true
		}
	}
	return s
}

func run() error {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println("randr: watching for monitor changes...")

	prev, err := parseXrandr()
	if err != nil {
		return err
	}
	prevSet := connectedSet(prev)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			log.Println("randr: shutting down")
			return nil
		case <-ticker.C:
		}

		cur, err := parseXrandr()
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}
		curSet := connectedSet(cur)

		// Detect newly connected outputs.
		var newOutputs []string
		for name := range curSet {
			if !prevSet[name] {
				newOutputs = append(newOutputs, name)
			}
		}

		if len(newOutputs) > 0 {
			log.Printf("new monitor(s) detected: %s", strings.Join(newOutputs, ", "))

			// Identify primary and all connected externals.
			var primary output
			var externals []output
			var all []output
			for _, o := range cur {
				if !o.Connected {
					continue
				}
				if o.Primary {
					primary = o
				} else {
					externals = append(externals, o)
				}
				all = append(all, o)
			}

			if primary.Name == "" && len(all) > 0 {
				primary = all[0]
				externals = all[1:]
			}

			if len(externals) > 0 {
				res := bestCommonResolution(all)
				log.Printf("mirroring at %s", res)
				if err := mirror(primary, externals, res); err != nil {
					log.Printf("mirror failed: %v", err)
				}
			}
		}

		// Detect disconnected outputs — revert primary to its native res.
		var removed []string
		for name := range prevSet {
			if !curSet[name] {
				removed = append(removed, name)
			}
		}
		if len(removed) > 0 {
			log.Printf("monitor(s) disconnected: %s", strings.Join(removed, ", "))
			for _, o := range cur {
				if o.Connected && o.Primary && len(o.Resolutions) > 0 {
					native := o.Resolutions[0]
					log.Printf("restoring %s to native %s", o.Name, native)
					cmd := exec.Command("xrandr",
						"--output", o.Name,
						"--mode", native.String(),
						"--primary",
					)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						log.Printf("restore failed: %v", err)
					}
					break
				}
			}
		}

		prevSet = curSet
		prev = cur
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "randr: %v\n", err)
		os.Exit(1)
	}
}
