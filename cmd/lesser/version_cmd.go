package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

func printVersionTo(w io.Writer) {
	buildInfo, ok := debug.ReadBuildInfo()
	_, _ = io.WriteString(w, formatVersion(buildInfo, ok))
}

func formatVersion(buildInfo *debug.BuildInfo, ok bool) string {
	if !ok || buildInfo == nil {
		return "lesser (unknown)\n"
	}

	version := strings.TrimSpace(buildInfo.Main.Version)
	if version == "" {
		version = "(unknown)"
	}

	revision, modified := vcsInfo(buildInfo.Settings)
	if revision == "" {
		return fmt.Sprintf("lesser %s\n", version)
	}

	short := revision
	if len(short) > 12 {
		short = short[:12]
	}

	if modified {
		return fmt.Sprintf("lesser %s (%s, dirty)\n", version, short)
	}
	return fmt.Sprintf("lesser %s (%s)\n", version, short)
}

func vcsInfo(settings []debug.BuildSetting) (string, bool) {
	revision := ""
	modified := false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = strings.TrimSpace(setting.Value) == "true"
		}
	}
	return revision, modified
}
