package buildinfo

import "runtime/debug"

const Version = "0.1.1"

func Commit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

func Display() string {
	return Version + " (" + Commit() + ")"
}
