package main

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// Service describes how a server app runs as a system service. Values in
// Configuration may use {configuration}, {state} and {cache}: the platform
// directories of the app. Packages install Configuration as
// <configuration>/<ConfigurationFile>, owned by the service user.
type Service struct {
	Arguments         []string          `json:"arguments"`
	ConfigurationFile string            `json:"configuration_file"`
	Configuration     map[string]string `json:"configuration"`
}

// servicePlatform holds the directories of one app on one platform.
type servicePlatform struct {
	Binaries      string
	Configuration string
	State         string
	Cache         string
}

func linuxPlatform(name string) servicePlatform {
	return servicePlatform{
		Binaries:      "/usr/bin",
		Configuration: "/etc/" + name,
		State:         "/var/lib/" + name,
		Cache:         "/var/cache/" + name,
	}
}

func freebsdPlatform(name string) servicePlatform {
	return servicePlatform{
		Binaries:      "/usr/local/bin",
		Configuration: "/usr/local/etc/" + name,
		State:         "/var/db/" + name,
		Cache:         "/var/cache/" + name,
	}
}

var (
	argumentPattern          = regexp.MustCompile(`^[A-Za-z0-9._=/:-]+$`)
	configurationFilePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	configurationKeyPattern  = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	// Values are written unquoted to a dotenv file, so they carry no
	// whitespace, quotes, comments or expansions.
	configurationValuePattern = regexp.MustCompile(`^[^\s"'#$\\{}]*$`)
)

// validate checks that every value renders safely in sh, systemd and dotenv files.
func (service Service) validate() error {
	if len(service.Arguments) == 0 {
		return fmt.Errorf("service has no arguments")
	}
	for _, argument := range service.Arguments {
		if !argumentPattern.MatchString(argument) {
			return fmt.Errorf("service argument %q has characters that need quoting", argument)
		}
	}
	if !configurationFilePattern.MatchString(service.ConfigurationFile) || strings.Trim(service.ConfigurationFile, ".") == "" {
		return fmt.Errorf("invalid service configuration_file %q", service.ConfigurationFile)
	}
	for key := range service.Configuration {
		if !configurationKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid service configuration key %q", key)
		}
	}
	_, err := service.configurationFor("app", servicePlatform{Configuration: "/c", State: "/s", Cache: "/k"})
	return err
}

// configurationFor renders the configuration file for one platform.
func (service Service) configurationFor(name string, platform servicePlatform) (string, error) {
	directories := strings.NewReplacer(
		"{configuration}", platform.Configuration,
		"{state}", platform.State,
		"{cache}", platform.Cache,
	)
	keys := make([]string, 0, len(service.Configuration))
	for key := range service.Configuration {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Package configuration. The service and `sudo %s` read this file.\n", name)
	for _, key := range keys {
		value := directories.Replace(service.Configuration[key])
		if !configurationValuePattern.MatchString(value) {
			return "", fmt.Errorf("service configuration %s: value %q has unknown placeholders or characters that need quoting", key, service.Configuration[key])
		}
		fmt.Fprintf(&builder, "%s=%s\n", key, value)
	}
	return builder.String(), nil
}

//go:embed templates
var templateFiles embed.FS

// Square brackets keep the templates readable: shell and Nix use braces.
var serviceTemplates = template.Must(template.New("").Delims("[[", "]]").Funcs(template.FuncMap{
	"shell":   shellString,
	"systemd": systemdString,
	"join":    strings.Join,
}).ParseFS(templateFiles, "templates/*"))

type serviceData struct {
	Release
	Platform servicePlatform
	// Command is the binary path followed by the service arguments.
	Command string
	// RCName is the FreeBSD rc.d name: rc variables allow no dashes.
	RCName string
	// Restart adds a restart of a running service to an install script.
	Restart bool
}

// renderService adds the service files of one release to files. Releases
// without a service add nothing.
func renderService(release Release, files map[string][]byte) error {
	if release.Service == nil {
		return nil
	}
	directory := filepath.Join("services", release.Name)
	linux := linuxPlatform(release.Name)
	freebsd := freebsdPlatform(release.Name)
	newData := func(platform servicePlatform, restart bool) serviceData {
		return serviceData{
			Release:  release,
			Platform: platform,
			Command:  strings.Join(append([]string{platform.Binaries + "/" + release.Binary}, release.Service.Arguments...), " "),
			RCName:   strings.ReplaceAll(release.Name, "-", "_"),
			Restart:  restart,
		}
	}

	for platformName, platform := range map[string]servicePlatform{"linux": linux, "freebsd": freebsd} {
		configuration, err := release.Service.configurationFor(release.Name, platform)
		if err != nil {
			return err
		}
		files[filepath.Join(directory, platformName, "configuration")] = []byte(configuration)
	}

	outputs := []struct {
		file     string
		template string
		data     serviceData
	}{
		{"linux/" + release.Name + ".service", "systemd.service", newData(linux, false)},
		{"linux/postinstall.sh", "systemd-postinstall.sh", newData(linux, false)},
		{"linux/preremove.sh", "systemd-preremove.sh", newData(linux, false)},
		{"linux/postremove.sh", "systemd-postremove.sh", newData(linux, false)},
		{"alpine/" + release.Name + ".initd", "openrc.initd", newData(linux, false)},
		{"alpine/preinstall.sh", "openrc-preinstall.sh", newData(linux, false)},
		{"alpine/postinstall.sh", "openrc-install.sh", newData(linux, false)},
		{"alpine/postupgrade.sh", "openrc-install.sh", newData(linux, true)},
		{"alpine/preremove.sh", "openrc-preremove.sh", newData(linux, false)},
		{"freebsd/" + newData(freebsd, false).RCName, "freebsd.rc", newData(freebsd, false)},
		{"freebsd/post-install.sh", "freebsd-post-install.sh", newData(freebsd, false)},
	}
	for _, output := range outputs {
		var buffer bytes.Buffer
		if err := serviceTemplates.ExecuteTemplate(&buffer, output.template, output.data); err != nil {
			return fmt.Errorf("%s: %w", output.template, err)
		}
		files[filepath.Join(directory, filepath.FromSlash(output.file))] = buffer.Bytes()
	}
	return nil
}

// systemdString escapes the "%" specifier character of systemd unit files.
func systemdString(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

// shellString quotes a value as a single-quoted sh literal.
func shellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
