package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// --- Structures ---

type Branch struct {
	Name   string `json:"name"`
	Commit struct {
		Commit struct {
			Author struct {
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	} `json:"commit"`
}

type FileContent struct {
	Content string `json:"content"`
}

type Pubspec struct {
	Dependencies        map[string]interface{} `yaml:"dependencies"`
	DevDependencies     map[string]interface{} `yaml:"dev_dependencies"`
	DependencyOverrides map[string]interface{} `yaml:"dependency_overrides"`
}

type Stats struct {
	Dependencies        map[string]map[string]interface{} `json:"dependencies"`
	DevDependencies     map[string]map[string]interface{} `json:"dev_dependencies"`
	DependencyOverrides map[string]map[string]interface{} `json:"dependency_overrides"`
}

// --- Core logic ---

func getLatestBranch(ctx context.Context, client *http.Client, owner, repo, token string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches", owner, repo)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get branches: %s (%s)", resp.Status, string(body))
	}

	var branches []Branch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return "", err
	}
	if len(branches) == 0 {
		return "", fmt.Errorf("no branches found")
	}

	sort.Slice(branches, func(i, j int) bool {
		return branches[i].Commit.Commit.Author.Date.After(branches[j].Commit.Commit.Author.Date)
	})

	return branches[0].Name, nil
}

func getPubspec(ctx context.Context, client *http.Client, owner, repo, branch, token string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/pubspec.yaml?ref=%s", owner, repo, branch)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch pubspec.yaml from %s/%s (%s)", owner, repo, resp.Status)
	}

	var file FileContent
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parsePubspec(content string) Pubspec {
	var ps Pubspec
	_ = yaml.Unmarshal([]byte(content), &ps)
	return ps
}

// --- Main logic ---

func main() {
	envPath := flag.String("env", "", "Path to .env file containing GITHUB_TOKEN")
	reposPath := flag.String("repos", "", "Path to file with list of GitHub repositories")
	outPath := flag.String("out", "", "Path to output JSON file")
	minUsage := flag.Int("min", 1, "Minimum usage count for package to be included in statistics")
	helpFlag := flag.Bool("help", false, "Show usage help")
	flag.Parse()

	if *helpFlag {
		fmt.Println(`Usage:
  pgs --env .env --repos repos.txt --out stats.json [--min N]

Options:
  --env     Path to .env file containing GITHUB_TOKEN
  --repos   Path to file with GitHub repositories (format: owner/repo per line)
  --out     Path to output JSON file
  --min     Minimum number of package usages to include in stats (default: 1)
  --help    Show this help message`)
		return
	}

	if *envPath == "" || *reposPath == "" || *outPath == "" {
		fmt.Println("Missing required arguments. Use --help for usage.")
		return
	}

	_ = godotenv.Load(*envPath)
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("GITHUB_TOKEN not found in .env file")
		return
	}

	file, err := os.ReadFile(*reposPath)
	if err != nil {
		fmt.Printf("Failed to read repos file: %v\n", err)
		return
	}

	repos := strings.Fields(strings.TrimSpace(string(file)))
	if len(repos) == 0 {
		fmt.Println("No repositories found in the file.")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()
	mu := sync.Mutex{}

	type counter struct {
		deps      map[string]int
		devDeps   map[string]int
		overrides map[string]int
	}
	stats := counter{
		deps:      map[string]int{},
		devDeps:   map[string]int{},
		overrides: map[string]int{},
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, full := range repos {
		wg.Add(1)
		go func(i int, full string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(repos), full)
			parts := strings.Split(full, "/")
			if len(parts) != 2 {
				fmt.Printf("Invalid repo format: %s\n", full)
				return
			}
			owner, repo := parts[0], parts[1]

			branch, err := getLatestBranch(ctx, client, owner, repo, token)
			if err != nil {
				fmt.Printf("Error getting branch for %s: %v\n", full, err)
				return
			}

			content, err := getPubspec(ctx, client, owner, repo, branch, token)
			if err != nil {
				fmt.Printf("Error fetching pubspec.yaml for %s: %v\n", full, err)
				return
			}

			ps := parsePubspec(content)
			mu.Lock()
			for k := range ps.Dependencies {
				stats.deps[k]++
			}
			for k := range ps.DevDependencies {
				stats.devDeps[k]++
			}
			for k := range ps.DependencyOverrides {
				stats.overrides[k]++
			}
			mu.Unlock()
		}(i, full)
	}

	wg.Wait()

	// Filter by min usage
	finalStats := Stats{
		Dependencies:        map[string]map[string]interface{}{},
		DevDependencies:     map[string]map[string]interface{}{},
		DependencyOverrides: map[string]map[string]interface{}{},
	}

	for pkg, count := range stats.deps {
		if count >= *minUsage {
			finalStats.Dependencies[pkg] = map[string]interface{}{
				"count": count,
				"url":   fmt.Sprintf("https://pub.dev/packages/%s", pkg),
			}
		}
	}
	for pkg, count := range stats.devDeps {
		if count >= *minUsage {
			finalStats.DevDependencies[pkg] = map[string]interface{}{
				"count": count,
				"url":   fmt.Sprintf("https://pub.dev/packages/%s", pkg),
			}
		}
	}
	for pkg, count := range stats.overrides {
		if count >= *minUsage {
			finalStats.DependencyOverrides[pkg] = map[string]interface{}{
				"count": count,
				"url":   fmt.Sprintf("https://pub.dev/packages/%s", pkg),
			}
		}
	}

	data, _ := json.MarshalIndent(finalStats, "", "  ")
	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fmt.Printf("Failed to write JSON: %v\n", err)
		return
	}

	fmt.Printf("✅ Stats collected successfully (min usage %d)\n", *minUsage)
	fmt.Printf("Saved to %s\n", *outPath)
}
