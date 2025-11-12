# PubScan

A tool for collecting dependency statistics from Flutter/Dart projects on GitHub.

## Description

PubScan analyzes `pubspec.yaml` files in GitHub repositories and collects package usage statistics. The utility connects to the GitHub API, downloads project configuration files, and counts the frequency of usage of various libraries.

## Features

- 📊 Collect dependency statistics from multiple repositories
- ⚡ Parallel processing with configurable request limit
- 🔒 Secure GitHub API integration with tokens
- 📝 Export results to JSON format
- 🛡️ Rate limiting protection
- 📋 Detailed process logging

## Installation

1. Clone the repository:
```bash
git clone https://github.com/plasmatrip/pubscan.git
cd pubscan
```

2. Install dependencies:
```bash
go mod download
```

3. Build the project:
```bash
go build -o bin/pubscan cmd/main.go
```

## Usage

### Setup

1. Create a file with GitHub token (e.g., `.env`):
```bash
GITHUB_TOKEN=ghp_your_token_here
```

2. Prepare a file with repository list (e.g., `repos.txt`):
```
flutter/flutter
dart-lang/dart
google/flutter-desktop-embedding
```

### Running

```bash
./bin/pubscan --env .env --repos repos.txt --out stats.json --limit 5
```

### Command Line Parameters

| Parameter | Description | Required |
|-----------|-------------|----------|
| `--env` | Path to file with GitHub token | ✅ |
| `--repos` | Path to file with repository list | ✅ |
| `--out` | Path to output JSON file | ✅ |
| `--limit` | Maximum number of parallel requests (default: 5) | ❌ |
| `--help` | Show help message | ❌ |

## Output Format

Results are saved to a JSON file in the following format:

```json
{
  "flutter": 45,
  "cupertino_icons": 38,
  "provider": 25,
  "http": 22,
  "shared_preferences": 18
}
```

Where the key is the package name and the value is the number of repositories that use it.

## Requirements

- Go 1.24.0 or higher
- GitHub Personal Access Token
- Internet access

## Getting GitHub Token

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Create a new token with `public_repo` permissions (for public repositories)
3. Copy the token and save it in the `.env` file

## Limitations

- Supports only public repositories
- Analyzes only `pubspec.yaml` files in the project root
- Subject to GitHub API rate limiting

## Dependencies

- `github.com/google/go-github/v66` - GitHub API client
- `golang.org/x/oauth2` - OAuth2 authentication
- `gopkg.in/yaml.v3` - YAML parser

## License

MIT

## Contributing

Pull requests and issues are welcome! Please follow standard Go code formatting practices.