package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type target struct {
	OS   string
	Arch string
}

func main() {
	platforms := flag.String("platforms", "linux,darwin,windows", "A comma-separated list of platforms to build for (e.g., linux,darwin,windows)")
	flag.Parse()

	targets := []target{}
	for _, p := range strings.Split(*platforms, ",") {
		switch p {
		case "linux":
			targets = append(targets, target{OS: "linux", Arch: "amd64"})
		case "darwin":
			targets = append(targets, target{OS: "darwin", Arch: "amd64"})
		case "windows":
			targets = append(targets, target{OS: "windows", Arch: "amd64"})
		default:
			log.Printf("unknown platform: %s", p)
		}
	}

	for _, t := range targets {
		fmt.Printf("Building for %s/%s...\n", t.OS, t.Arch)
		ldflags := fmt.Sprintf("-X main.version=%s -X main.commit=%s -X 'main.date=%s'", version, commit, date)
		outputName := fmt.Sprintf("./build/dreamfs-%s-%s", t.OS, t.Arch)
		if t.OS == "windows" {
			outputName += ".exe"
		}

		cmd := exec.Command("go", "build", "-ldflags="+ldflags, "-o", outputName, "./cmd/indexer")
		cmd.Env = append(os.Environ(), "GOOS="+t.OS, "GOARCH="+t.Arch)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatalf("failed to build for %s/%s: %v\n%s", t.OS, t.Arch, err, output)
		}
		fmt.Printf("Successfully built %s\n", outputName)
	}
}
