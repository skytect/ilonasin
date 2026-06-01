package cli

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"
)

var (
	Version       = "0.1.0"
	Commit        = ""
	CommitSubject = ""
)

func VersionString() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	commit, modified := vcsRevision()
	if strings.TrimSpace(Commit) != "" {
		commit = strings.TrimSpace(Commit)
	}
	subject := strings.TrimSpace(CommitSubject)
	if subject == "" {
		subject = localGitCommitSubject()
	}
	details := make([]string, 0, 3)
	if commit != "" {
		details = append(details, shortCommit(commit))
	}
	if subject != "" {
		details = append(details, subject)
	}
	if modified {
		details = append(details, "modified")
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

func localGitCommitSubject() string {
	if !looksLikeIloRepo() {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "log", "-1", "--format=%s").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func looksLikeIloRepo() bool {
	body, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "module ilonasin" {
			return true
		}
	}
	return false
}
