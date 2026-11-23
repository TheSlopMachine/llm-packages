// Command generate resolves the latest release of every app in apps.json and
// rewrites the generated directories: releases/, bucket/, Formula/ and services/.
//
// releases/<app>.json is the release record. The Nix flake and the repository
// scripts read it. bucket/, Formula/ and services/ are rendered from the same
// record. services/<app>/ holds the service files of server apps.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// App is one entry of apps.json.
type App struct {
	Name        string `json:"name"`
	Repository  string `json:"repository"`
	Binary      string `json:"binary"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	License     string `json:"license"`
	// Service is set for server apps. Packages install it as a system service.
	Service *Service `json:"service,omitempty"`
}

// Asset is one release archive. The key in Release.Assets is "<goos>_<goarch>".
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Release is the record written to releases/<app>.json.
type Release struct {
	App
	Version string           `json:"version"`
	Tag     string           `json:"tag"`
	Assets  map[string]Asset `json:"assets"`
}

// generatedDirectories are owned by this command. Files not produced by a run are deleted.
var generatedDirectories = []string{"releases", "bucket", "Formula", "services"}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apps, err := loadApps("apps.json")
	if err != nil {
		return err
	}
	client := newGitHubClient(os.Getenv("GITHUB_TOKEN"))

	files := map[string][]byte{}
	for _, app := range apps {
		release, err := client.latestRelease(app)
		if err != nil {
			return fmt.Errorf("%s: %w", app.Name, err)
		}
		fmt.Printf("%s %s (%d assets)\n", app.Name, release.Version, len(release.Assets))
		if err := renderRelease(release, files); err != nil {
			return fmt.Errorf("%s: %w", app.Name, err)
		}
	}
	return writeGenerated(files)
}

func loadApps(path string) ([]App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document struct {
		Apps []App `json:"apps"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(document.Apps) == 0 {
		return nil, fmt.Errorf("%s: no apps", path)
	}
	seen := map[string]bool{}
	for _, app := range document.Apps {
		if !namePattern.MatchString(app.Name) {
			return nil, fmt.Errorf("%s: invalid app name %q", path, app.Name)
		}
		if seen[app.Name] {
			return nil, fmt.Errorf("%s: duplicate app %q", path, app.Name)
		}
		seen[app.Name] = true
		if app.Repository == "" || app.Binary == "" || app.Description == "" || app.Homepage == "" || app.License == "" {
			return nil, fmt.Errorf("%s: app %q misses a required field", path, app.Name)
		}
		if app.Service != nil {
			if err := app.Service.validate(); err != nil {
				return nil, fmt.Errorf("%s: app %q: %w", path, app.Name, err)
			}
		}
	}
	return document.Apps, nil
}

// renderRelease adds every file generated for one release to files, keyed by relative path.
func renderRelease(release Release, files map[string][]byte) error {
	record, err := json.MarshalIndent(release, "", "    ")
	if err != nil {
		return err
	}
	files[filepath.Join("releases", release.Name+".json")] = append(record, '\n')

	scoop, err := renderScoop(release)
	if err != nil {
		return fmt.Errorf("scoop: %w", err)
	}
	if scoop != nil {
		files[filepath.Join("bucket", release.Name+".json")] = scoop
	}

	formula, err := renderFormula(release)
	if err != nil {
		return fmt.Errorf("homebrew: %w", err)
	}
	if formula != nil {
		files[filepath.Join("Formula", release.Name+".rb")] = formula
	}

	if err := renderService(release, files); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

// writeGenerated writes files and deletes stale files and empty directories in the generated directories.
func writeGenerated(files map[string][]byte) error {
	for _, directory := range generatedDirectories {
		var directories []string
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				directories = append(directories, path)
				return nil
			}
			if _, keep := files[path]; keep {
				return nil
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			fmt.Printf("removed %s\n", filepath.ToSlash(path))
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// Walk order puts parents first, so the reverse order empties children first.
		for index := len(directories) - 1; index >= 0; index-- {
			entries, err := os.ReadDir(directories[index])
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				if err := os.Remove(directories[index]); err != nil {
					return err
				}
			}
		}
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[path], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// versionFromTag strips the "v" prefix. Only dotted numbers are accepted:
// deb, rpm, apk and FreeBSD pkg all parse them the same way.
func versionFromTag(tag string) (string, error) {
	version := strings.TrimPrefix(tag, "v")
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("tag %q is not a dotted numeric version", tag)
	}
	return version, nil
}
