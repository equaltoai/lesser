package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

func printVersionTo(w io.Writer) {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		_, _ = fmt.Fprintln(w, "lesser (unknown)")
		return
	}

	version := strings.TrimSpace(buildInfo.Main.Version)
	if version == "" {
		version = "(unknown)"
	}

	revision := ""
	modified := ""
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = strings.TrimSpace(setting.Value)
		}
	}

	if revision == "" {
		_, _ = fmt.Fprintf(w, "lesser %s\n", version)
		return
	}

	short := revision
	if len(short) > 12 {
		short = short[:12]
	}
	if modified == "true" {
		_, _ = fmt.Fprintf(w, "lesser %s (%s, dirty)\n", version, short)
		return
	}
	_, _ = fmt.Fprintf(w, "lesser %s (%s)\n", version, short)
}
