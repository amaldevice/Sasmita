package repositories_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
)

func seedShellData(t *testing.T) *repositories.Store {
	t.Helper()
	store := newStore(t, filepath.Join(t.TempDir(), "sasmita.db"))
	ctx := t.Context()
	now := time.Now()
	projects := []domain.Project{
		{ID: "p1", Name: "Work", RootURI: "file:///work", CreatedAt: now, UpdatedAt: now},
		{ID: "p2", Name: "Personal", RootURI: "", CreatedAt: now, UpdatedAt: now},
	}
	for i := range projects {
		if err := store.CreateProject(ctx, &projects[i]); err != nil {
			t.Fatal(err)
		}
	}
	convs := []domain.Conversation{
		{ID: "c1", ProjectID: "p1", Title: "Context notes", CreatedAt: now, UpdatedAt: now},
		{ID: "c2", ProjectID: "p2", Title: "Budget recap", CreatedAt: now, UpdatedAt: now},
		// A conversation with no project: project_id is the sentinel "" row,
		// whose root_uri is empty, so there is no folder to open.
		{ID: "c3", ProjectID: "", Title: "Loose thought", CreatedAt: now, UpdatedAt: now},
	}
	for i := range convs {
		if err := store.CreateConversation(ctx, &convs[i]); err != nil {
			t.Fatal(err)
		}
	}
	msgs := []domain.Message{
		{ID: "m1", ConversationID: "c1", Role: domain.RoleUser, Content: domain.TextContent("what changed in the context note"), CreatedAt: now},
		{ID: "m2", ConversationID: "c2", Role: domain.RoleAssistant, Content: domain.TextContent("compaction keeps the budget summary small"), CreatedAt: now},
	}
	for i := range msgs {
		if err := store.AddMessage(ctx, &msgs[i]); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestSearchConversationsOverTitlesAndMessages(t *testing.T) {
	store := seedShellData(t)
	ctx := t.Context()

	cases := []struct {
		query string
		want  string
	}{
		{"context", "c1"},      // title
		{"notes", "c1"},        // title token, prefix search
		{"changed", "c1"},      // message text
		{"compaction", "c2"},   // message text only
		{"budget", "c2"},       // title and message
		{"loose", "c3"},        // conversation with no project
		{"does-not-exist", ""}, // no match
	}
	for _, tc := range cases {
		results, err := store.SearchConversations(ctx, tc.query, 10)
		if err != nil {
			t.Fatalf("SearchConversations(%q): %v", tc.query, err)
		}
		ids := make([]string, 0, len(results))
		for _, r := range results {
			ids = append(ids, r.ConversationID)
			if r.Title == "" {
				t.Errorf("SearchConversations(%q) result has no title", tc.query)
			}
		}
		if tc.want == "" && len(ids) != 0 {
			t.Errorf("SearchConversations(%q) = %v, want no results", tc.query, ids)
		}
		if tc.want != "" && (len(ids) == 0 || ids[0] != tc.want) {
			t.Errorf("SearchConversations(%q) = %v, want top result %q", tc.query, ids, tc.want)
		}
	}

	// FTS snippets come from the matched message, not the raw JSON column.
	results, err := store.SearchConversations(ctx, "compaction", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || !contains(results[0].Snippet, "compaction") {
		t.Fatalf("snippet = %q, want matched text", results)
	}
}

func TestListRecentConversationsAcrossProjects(t *testing.T) {
	store := seedShellData(t)
	recent, next, err := store.ListRecentConversations(t.Context(), "", 10)
	if err != nil || next != "" || len(recent) != 3 {
		t.Fatalf("ListRecentConversations = %+v, next=%q (err %v)", recent, next, err)
	}
	// Most recently updated first.
	if recent[0].ID != "c3" {
		t.Fatalf("recent[0] = %q, want c3", recent[0].ID)
	}
}

func TestRenameAndMoveConversation(t *testing.T) {
	store := seedShellData(t)
	ctx := t.Context()
	now := time.Now()

	if err := store.RenameConversation(ctx, "c1", "Renamed title", now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetConversation(ctx, "c1")
	if err != nil || got.Title != "Renamed title" {
		t.Fatalf("conversation after rename = %+v (err %v)", got, err)
	}
	results, err := store.SearchConversations(ctx, "renamed", 10)
	if err != nil || len(results) == 0 || results[0].ConversationID != "c1" {
		t.Fatalf("search after rename = %+v (err %v)", results, err)
	}

	if err := store.MoveConversation(ctx, "c1", "p2", now); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetConversation(ctx, "c1")
	if err != nil || got.ProjectID != "p2" {
		t.Fatalf("conversation after move = %+v (err %v)", got, err)
	}

	// Moving to no project must keep the conversation reachable but with an
	// empty project id; the sentinel project is not a user-visible project.
	if err := store.MoveConversation(ctx, "c1", "", now); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetConversation(ctx, "c1")
	if err != nil || got.ProjectID != "" {
		t.Fatalf("conversation after move to no project = %+v (err %v)", got, err)
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.ID == "" {
			t.Fatal("sentinel no-project row leaked into ListProjects")
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
