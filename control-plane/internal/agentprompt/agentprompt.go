// Package agentprompt is the single source of truth for the platform system
// prompt injected into every coding-agent run. The prompt text lives in the
// committed prompt.md and is embedded into BOTH binaries that need it:
//   - cmd/runtimed injects it into the agent (claude --append-system-prompt /
//     opencode prompt preamble), rendered with the sandbox's real port so no
//     loopback address or port is hard-coded in the source.
//   - internal/api serves the rendered text (with defaults) in GET /v1/settings
//     so the console can display it read-only.
package agentprompt

import (
	_ "embed"
	"strconv"
	"strings"
)

//go:embed prompt.md
var raw string

// Defaults used when a field is unset (also what the read-only Settings view
// shows). Kept here, not in prompt.md, so the template carries no literal
// loopback address or port.
const (
	defaultAppDir     = "/home/sandbox/workspace/app"
	defaultPort       = 3000
	defaultHealthPath = "/"
	loopbackHost      = "127.0.0.1" // the sandbox's own loopback — always correct in-container
)

// Vars are the per-sandbox values substituted into the template.
type Vars struct {
	AppDir     string
	Port       int
	HealthPath string
	// Supabase is the rendered project section, or empty when this project has
	// no Supabase connection. It must not contain a token or proxy secret.
	Supabase string
}

// Raw returns the unrendered template (with {{PLACEHOLDERS}}), for tests/debug.
func Raw() string { return raw }

// Render substitutes the sandbox's real values into the template. Any zero
// field falls back to the documented default.
func Render(v Vars) string {
	appDir := v.AppDir
	if appDir == "" {
		appDir = defaultAppDir
	}
	port := v.Port
	if port <= 0 {
		port = defaultPort
	}
	health := v.HealthPath
	if health == "" {
		health = defaultHealthPath
	}
	local := "http://" + loopbackHost + ":" + strconv.Itoa(port)
	return strings.NewReplacer(
		"{{APP_DIR}}", appDir,
		"{{PORT}}", strconv.Itoa(port),
		"{{HEALTH_PATH}}", health,
		"{{LOCAL_URL}}", local,
		"{{SUPABASE}}", strings.TrimRight(v.Supabase, "\n"),
	).Replace(raw)
}

// SupabaseSection is the briefing added only when a project has a linked
// Supabase connection. areas are the Supabase product areas the agent may
// change. apiURL and anonKey are the public client coordinates; anonKey is
// empty when they could not be loaded. Neither value is the access token.
func SupabaseSection(projectRef string, areas []string, apiURL, anonKey string) string {
	if projectRef == "" || len(areas) == 0 {
		return ""
	}
	if apiURL == "" {
		apiURL = "https://" + projectRef + ".supabase.co"
	}
	list := strings.Join(areas, ", ")
	var b strings.Builder
	b.WriteString("\n## Supabase\n\n")
	b.WriteString("This project is already connected to Supabase project `" + projectRef + "`.\n")
	b.WriteString("Project URL: " + apiURL + "\n")
	if anonKey != "" {
		b.WriteString("Publishable key: " + anonKey + "\n")
		b.WriteString("Write this URL and publishable key into the application when it needs to call Supabase. Do not ask the user for them, and do not repeat the publishable key in your reply.\n")
	} else {
		b.WriteString("The publishable key could not be loaded. Do not invent one, and do not ask the user for an access token.\n")
	}
	b.WriteString("The user is not technical. Edit the application, including its screens. Also change the Supabase project itself when the request needs " + list + ". Use the Supabase tools already connected to this session.\n")
	b.WriteString("For Google or Apple sign-in, ask the user to paste the provider client id and client secret into the chat. As soon as both values are present, enable that provider with enable_google_sign_in or update_auth_config. The Google client must allow redirect " + apiURL + "/auth/v1/callback. Do not add a sign-in button for a provider you have not enabled. Do not send the user to a dashboard.\n")
	b.WriteString("Never print tokens, client secrets, or service-role keys. You do not have the service-role key.\n")
	return b.String()
}
