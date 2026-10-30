package agentprompt

import (
	"strings"
	"testing"
)

// The platform briefing must carry the Project-brain convention: agents are
// told to read workspace/BRAIN.md before starting and append durable learnings
// before finishing. Every agent adapter delivers this briefing, so this single
// section is what standardizes the brain across OpenCode/Claude Code/Codex.
func TestRenderIncludesProjectBrain(t *testing.T) {
	out := Render(Vars{AppDir: "/home/sandbox/workspace/app"})

	for _, want := range []string{
		"## Project brain",
		"/home/sandbox/workspace/app/BRAIN.md", // path rendered with the real app dir
		"verification command",
		"never write secrets",
		"If nothing durable was learned, write nothing.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered briefing missing %q", want)
		}
	}

	if strings.Contains(out, "{{APP_DIR}}") || strings.Contains(out, "{{SUPABASE}}") {
		t.Error("unsubstituted placeholder left in rendered briefing")
	}
	if strings.Contains(out, "## Supabase") {
		t.Error("supabase section must be absent until a project connection is rendered")
	}
}

func TestSupabaseSectionNamesOnlyGrantedAreas(t *testing.T) {
	out := Render(Vars{Supabase: SupabaseSection("abcd1234", []string{"database", "authentication"}, "", "sb_publishable_test")})
	for _, want := range []string{
		"## Supabase",
		"abcd1234",
		"https://abcd1234.supabase.co",
		"sb_publishable_test",
		"Edit the application",
		"change the Supabase project itself",
		"enable_google_sign_in",
		"https://abcd1234.supabase.co/auth/v1/callback",
		"database, authentication",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, "You may change only") || strings.Contains(out, "stop that part") {
		t.Fatal("the briefing must not tell the agent to stop or avoid the application")
	}
	if SupabaseSection("", []string{"database"}, "", "") != "" || SupabaseSection("abcd1234", nil, "", "") != "" {
		t.Fatal("empty connection must not produce a section")
	}
}
