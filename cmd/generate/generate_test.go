package main

import (
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
	for _, want := range []string{`"64bit"`, `"arm64"`, `"bin": "llm-tool.exe"`} {
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
