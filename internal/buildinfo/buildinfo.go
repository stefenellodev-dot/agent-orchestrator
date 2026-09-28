// Package buildinfo exposes the running binary's identity so an obsolete build
// can never run silently.
package buildinfo

// These are overridden at build time via -ldflags "-X".
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info is the externally visible identity of the running orchestrator.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	OpenCode  string `json:"opencode,omitempty"`
	Agents    string `json:"opencode_agents,omitempty"`
}

// Current returns the build identity (without OpenCode details).
func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}

func (i Info) String() string {
	return "version=" + i.Version + " commit=" + i.Commit + " built=" + i.BuildTime
}
