package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

type Pubspec struct {
	Dependencies    map[string]interface{} `yaml:"dependencies"`
	DevDependencies map[string]interface{} `yaml:"dev_dependencies"`
}

func main() {
	reposFile := flag.String("repos", "", "Path to file with list of repositories (<owner>/<repo> per line)")
	outFile := flag.String("out", "", "Path to JSON output file")
	envFile := flag.String("env", "", "Path to .env file with GITHUB_TOKEN")
	limit := flag.Int("limit", 5, "Maximum number of parallel requests (default: 5)")
	help := flag.Bool("help", false, "Show usage help")

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	if *reposFile == "" || *outFile == "" || *envFile == "" {
		fmt.Println("Usage: go run . --env <path> --repos <path> --out <path> [--limit N]")
		fmt.Println("Run with --help to see all options.")
		os.Exit(1)
	}

	token, err := readEnvToken(*envFile)
	if err != nil {
		log.Fatalf("Failed to read GITHUB_TOKEN: %v", err)
	}
	if token == "" {
		log.Fatal("GITHUB_TOKEN not found in env file")
	}

	repos, err := readRepos(*reposFile)
	if err != nil {
		log.Fatalf("Failed to read repos file: %v", err)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	allDeps := collectDependencies(ctx, client, repos, *limit)

	if err := writeJSON(*outFile, allDeps); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	fmt.Printf("✅ Stats written to %s (%d libraries)\n", *outFile, len(allDeps))
}

func printHelp() {
	fmt.Println(`GitHub pubspec.yaml stats collector

Usage:
  --env <path>     Path to .env file containing GITHUB_TOKEN
  --repos <path>   Path to a file with GitHub repositories (one per line)
  --out <path>     Path to JSON file to write the result
  --limit <n>      Limit of concurrent requests (default: 5)
  --help           Show this help message

Example:
  go run . --env .env --repos repos.txt --out stats.json --limit 3

Env file example:
  GITHUB_TOKEN=ghp_ABC123xyz`)
}

func readEnvToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "GITHUB_TOKEN=") {
			return strings.TrimPrefix(line, "GITHUB_TOKEN="), nil
		}
	}
	return "", scanner.Err()
}

func readRepos(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var repos []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			repos = append(repos, line)
		}
	}
	return repos, nil
}

func collectDependencies(ctx context.Context, client *github.Client, repos []string, limit int) map[string]int {
	var mu sync.Mutex
	allDeps := make(map[string]int)
	wg := sync.WaitGroup{}
	sem := make(chan struct{}, limit)

	for _, repoFull := range repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			sem <- struct{}{} // acquire slot
			defer func() { <-sem }()

			owner, repo, ok := splitRepo(r)
			if !ok {
				log.Printf("invalid repo format: %s", r)
				return
			}

			content, _, _, err := client.Repositories.GetContents(ctx, owner, repo, "pubspec.yaml", nil)
			if err != nil {
				log.Printf("[%s] failed to fetch pubspec.yaml: %v", r, err)
				return
			}

			data, err := content.GetContent()
			if err != nil {
				log.Printf("[%s] failed to read pubspec.yaml: %v", r, err)
				return
			}

			deps, err := extractDependencies([]byte(data))
			if err != nil {
				log.Printf("[%s] failed to parse yaml: %v", r, err)
				return
			}

			mu.Lock()
			for _, d := range deps {
				allDeps[d]++
			}
			mu.Unlock()

			time.Sleep(200 * time.Millisecond) // to avoid hitting rate limits
		}(repoFull)
	}

	wg.Wait()
	return allDeps
}

func extractDependencies(data []byte) ([]string, error) {
	var p Pubspec
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	var deps []string
	for k := range p.Dependencies {
		deps = append(deps, k)
	}
	for k := range p.DevDependencies {
		deps = append(deps, k)
	}
	return deps, nil
}

func splitRepo(full string) (owner, repo string, ok bool) {
	parts := strings.Split(full, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func writeJSON(path string, m map[string]int) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
