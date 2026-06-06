package cli

import (
	"runtime/debug"
	"strings"
)

var (
	Version       = "0.1.0"
	Commit        = ""
	CommitSubject = ""
)

func VersionString() string {
	version := strings.TrimSpace(Version)
	commit, modified := vcsRevision()
	if strings.TrimSpace(Commit) != "" {
		commit = strings.TrimSpace(Commit)
	}
	subject := strings.TrimSpace(CommitSubject)
	details := make([]string, 0, 3)
	if subject != "" {
		details = append(details, subject)
	}
	if modified {
		details = append(details, "modified")
	}
	if commit != "" {
		identity := shortCommit(commit)
		if len(details) == 0 {
			return identity
		}
		return identity + " (" + strings.Join(details, ", ") + ")"
	}
	if version == "" {
		version = "dev"
	}
	if len(details) == 0 {
		return version
	}
	return version + " (" + strings.Join(details, ", ") + ")"
}

func vcsRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
