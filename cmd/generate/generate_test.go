package main

import (
	"path/filepath"
	"strings"
	"testing"
)

const testBase = "https://github.com/example/tool/releases/download/v1.2/"

func TestParseChecksums(t *testing.T) {
	text := strings.Join([]string{
		strings.Repeat("a", 64) + "  linux_amd64/tool_linux_amd64.zip",
		strings.Repeat("b", 64) + "  windows_386/tool_windows_386.zip",
		strings.Repeat("c", 64) + "  other_linux_amd64.zip",
		strings.Repeat("d", 64) + "  notes.txt",
		"",
	}, "\n")
	assets, err := parseChecksums(text, "tool", testBase)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2: %v", len(assets), assets)
	}
	want := Asset{URL: testBase + "tool_linux_amd64.zip", SHA256: strings.Repeat("a", 64)}
	if assets["linux_amd64"] != want {
		t.Fatalf("linux_amd64 = %+v, want %+v", assets["linux_amd64"], want)
	}
}

func TestParseChecksumsRejects(t *testing.T) {
	cases := map[string]string{
		"malformed": "zz  tool_linux_amd64.zip",
		"duplicate": strings.Repeat("a", 64) + "  a/tool_linux_amd64.zip\n" + strings.Repeat("b", 64) + "  b/tool_linux_amd64.zip",
		"empty":     strings.Repeat("a", 64) + "  notes.txt",
	}
	for name, text := range cases {
		if _, err := parseChecksums(text, "tool", testBase); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestVersionFromTag(t *testing.T) {
	for tag, want := range map[string]string{"v0.12": "0.12", "v1.2.3": "1.2.3", "2.0": "2.0"} {
		got, err := versionFromTag(tag)
		if err != nil || got != want {
			t.Errorf("versionFromTag(%q) = %q, %v; want %q", tag, got, err, want)
		}
	}
	for _, tag := range []string{"v1.0.0-beta", "latest", "v"} {
		if _, err := versionFromTag(tag); err == nil {
			t.Errorf("versionFromTag(%q): expected an error", tag)
		}
	}
}

func testRelease(platforms ...string) Release {
	release := Release{
		App:     App{Name: "llm-tool", Binary: "llm-tool", Description: `Tool "#1"`, Homepage: "https://example.com", License: "MIT"},
		Version: "1.2",
		Tag:     "v1.2",
		Assets:  map[string]Asset{},
	}
	for _, platform := range platforms {
		release.Assets[platform] = Asset{URL: testBase + platform + ".zip", SHA256: platform}
	}
	return release
}

func TestRenderFormula(t *testing.T) {
	formula, err := renderFormula(testRelease("darwin_arm64", "linux_amd64", "linux_arm"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(formula)
	for _, want := range []string{
		"class LlmTool < Formula",
		`desc "Tool \"\#1\""`,
		"on_macos do\n    on_arm do\n      url \"" + testBase + "darwin_arm64.zip\"",
		"on_linux do\n    on_intel do",
		`bin.install "llm-tool"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("formula misses %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "on_macos do\n    on_intel") || strings.Contains(text, "linux_arm.zip") {
		t.Errorf("formula holds an archive it must skip:\n%s", text)
	}

	none, err := renderFormula(testRelease("windows_amd64"))
	if err != nil || none != nil {
		t.Errorf("Windows-only release: formula = %q, err = %v; want nil", none, err)
	}
}

func TestRenderScoop(t *testing.T) {
	manifest, err := renderScoop(testRelease("windows_amd64", "windows_arm64", "linux_amd64"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifest)
	for _, want := range []string{`"64bit"`, `"arm64"`, `"extract_to": "bin"`, `"bin": "bin\\llm-tool.exe"`, `$dir\\share\\llm-tool\\package-manager`} {
		if !strings.Contains(text, want) {
			t.Errorf("manifest misses %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"32bit"`) {
		t.Errorf("manifest holds a missing architecture:\n%s", text)
	}

	none, err := renderScoop(testRelease("linux_amd64"))
	if err != nil || none != nil {
		t.Errorf("Linux-only release: manifest = %q, err = %v; want nil", none, err)
	}
}

func testService() *Service {
	return &Service{
		Arguments:         []string{"serve"},
		ConfigurationFile: ".env",
		Configuration: map[string]string{
			"DB_PATH":  "{state}/tool.sqlite3",
			"KEY_FILE": "{configuration}/key",
		},
	}
}

func TestRenderFormulaService(t *testing.T) {
	release := testRelease("linux_amd64")
	release.Service = testService()
	formula, err := renderFormula(release)
	if err != nil {
		t.Fatal(err)
	}
	text := string(formula)
	for _, want := range []string{
		`(share/"llm-tool/package-manager").write "Homebrew\n"`,
		`run [opt_bin/"llm-tool", "serve"]`,
		`log_path var/"log/llm-tool.log"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("formula misses %q:\n%s", want, text)
		}
	}
}

func TestRenderService(t *testing.T) {
	release := testRelease("linux_amd64")
	release.Description = "Tool 100%"
	release.Service = testService()
	files := map[string][]byte{}
	if err := renderService(release, files); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"linux/configuration":     {"DB_PATH=/var/lib/llm-tool/tool.sqlite3\nKEY_FILE=/etc/llm-tool/key\n"},
		"freebsd/configuration":   {"DB_PATH=/var/db/llm-tool/tool.sqlite3\nKEY_FILE=/usr/local/etc/llm-tool/key\n"},
		"linux/llm-tool.service":  {"Description=Tool 100%%\n", "ExecStart=/usr/bin/llm-tool serve\n", "User=llm-tool\n"},
		"linux/postinstall.sh":    {"useradd --system --gid llm-tool --home-dir /var/lib/llm-tool", "chown llm-tool:llm-tool /etc/llm-tool /var/lib/llm-tool /var/cache/llm-tool /etc/llm-tool/.env\n", "systemctl try-restart llm-tool.service"},
		"linux/preremove.sh":      {"systemctl disable --now llm-tool.service"},
		"linux/postremove.sh":     {"systemctl daemon-reload"},
		"alpine/llm-tool.initd":   {"command=/usr/bin/llm-tool\n", "command_args='serve'\n", "export HOME=/var/lib/llm-tool\n"},
		"alpine/preinstall.sh":    {"adduser -S -D -H -h /var/lib/llm-tool"},
		"alpine/postinstall.sh":   {"chown llm-tool:llm-tool /var/lib/llm-tool /var/cache/llm-tool /etc/llm-tool/.env\n"},
		"alpine/postupgrade.sh":   {"rc-service --ifstarted llm-tool restart"},
		"alpine/preremove.sh":     {"rc-service --ifstarted llm-tool stop"},
		"freebsd/llm_tool":        {"name=llm_tool\n", `llm_tool_env="HOME=/var/db/llm-tool"`, "-T llm-tool /usr/local/bin/llm-tool serve\""},
		"freebsd/post-install.sh": {"pw useradd llm-tool -g llm-tool -d /var/db/llm-tool", "service llm_tool onerestart"},
	}
	if len(files) != len(want) {
		t.Errorf("rendered %d files, want %d: %v", len(files), len(want), keys(files))
	}
	for file, fragments := range want {
		content, ok := files[filepath.Join("services", "llm-tool", filepath.FromSlash(file))]
		if !ok {
			t.Errorf("missing %s", file)
			continue
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(content), fragment) {
				t.Errorf("%s misses %q:\n%s", file, fragment, content)
			}
		}
	}
	if content := string(files[filepath.Join("services", "llm-tool", "alpine", "postinstall.sh")]); strings.Contains(content, "rc-service") {
		t.Errorf("postinstall restarts the service:\n%s", content)
	}

	none := map[string][]byte{}
	if err := renderService(testRelease("linux_amd64"), none); err != nil || len(none) != 0 {
		t.Errorf("release without service: files = %v, err = %v; want none", keys(none), err)
	}
}

func TestServiceValidateRejects(t *testing.T) {
	cases := map[string]func(*Service){
		"no arguments":        func(service *Service) { service.Arguments = nil },
		"argument with space": func(service *Service) { service.Arguments = []string{"serve now"} },
		"file path":           func(service *Service) { service.ConfigurationFile = "../.env" },
		"dots only":           func(service *Service) { service.ConfigurationFile = ".." },
		"lowercase key":       func(service *Service) { service.Configuration["db"] = "x" },
		"unknown placeholder": func(service *Service) { service.Configuration["LOG"] = "{logs}/x" },
		"quote":               func(service *Service) { service.Configuration["NAME"] = `a"b` },
	}
	for name, change := range cases {
		service := testService()
		change(service)
		if err := service.validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := testService().validate(); err != nil {
		t.Errorf("valid service: %v", err)
	}
}

func keys(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, filepath.ToSlash(name))
	}
	return names
}
