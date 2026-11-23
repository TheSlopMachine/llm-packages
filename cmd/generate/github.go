package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type gitHubClient struct {
	http  *http.Client
	token string
}

func newGitHubClient(token string) *gitHubClient {
	return &gitHubClient{http: &http.Client{Timeout: 60 * time.Second}, token: token}
}

// latestRelease resolves the latest non-prerelease release and its archive checksums.
func (client *gitHubClient) latestRelease(app App) (Release, error) {
	body, err := client.get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", app.Repository))
	if err != nil {
		return Release{}, err
	}
	var latest struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &latest); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	version, err := versionFromTag(latest.TagName)
	if err != nil {
		return Release{}, err
	}

	downloadBase := fmt.Sprintf("https://github.com/%s/releases/download/%s/", app.Repository, latest.TagName)
	checksums, err := client.get(downloadBase + "checksums.txt")
	if err != nil {
		return Release{}, err
	}
	assets, err := parseChecksums(string(checksums), app.Binary, downloadBase)
	if err != nil {
		return Release{}, fmt.Errorf("checksums.txt of %s: %w", latest.TagName, err)
	}
	return Release{App: app, Version: version, Tag: latest.TagName, Assets: assets}, nil
}

func (client *gitHubClient) get(url string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if client.token != "" && strings.HasPrefix(url, "https://api.github.com/") {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	return body, nil
}

var (
	checksumLinePattern = regexp.MustCompile(`^([0-9a-f]{64})  (\S+)$`)
	archivePattern      = regexp.MustCompile(`^(.+)_([a-z0-9]+)_([a-z0-9]+)\.zip$`)
)

// parseChecksums reads "<sha256>  <dir>/<binary>_<goos>_<goarch>.zip" lines.
// Files of other shapes or other binaries are not archives of this app and are skipped.
func parseChecksums(text, binary, downloadBase string) (map[string]Asset, error) {
	assets := map[string]Asset{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		match := checksumLinePattern.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("line %d is malformed: %q", lineNumber, line)
		}
		fileName := match[2][strings.LastIndex(match[2], "/")+1:]
		archive := archivePattern.FindStringSubmatch(fileName)
		if archive == nil || archive[1] != binary {
			continue
		}
		platform := archive[2] + "_" + archive[3]
		if _, duplicate := assets[platform]; duplicate {
			return nil, fmt.Errorf("line %d repeats platform %s", lineNumber, platform)
		}
		assets[platform] = Asset{URL: downloadBase + fileName, SHA256: match[1]}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("no %s_<goos>_<goarch>.zip archives", binary)
	}
	return assets, nil
}
