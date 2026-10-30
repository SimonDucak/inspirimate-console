package store

import (
	"context"
	"testing"
)

func TestSupabaseConnectionStoreCRUDAndOwnerScope(t *testing.T) {
	ctx := context.Background()
	s := newGitCredStore(t)
	enc, nonce := []byte{0xde, 0xad}, []byte{0xbe, 0xef}

	a := &SupabaseConnection{ID: "01A", OwnerToken: "ownerA", Name: "Production", ProjectRef: "abcd1234", Features: `["database"]`}
	if err := s.CreateSupabaseConnection(ctx, a, enc, nonce); err != nil {
		t.Fatal(err)
	}
	dup := &SupabaseConnection{ID: "01B", OwnerToken: "ownerA", Name: "Production", ProjectRef: "zzzzzzzz", Features: `["database"]`}
	if err := s.CreateSupabaseConnection(ctx, dup, enc, nonce); err != ErrConflict {
		t.Fatalf("dup: got %v, want ErrConflict", err)
	}
	b := &SupabaseConnection{ID: "01C", OwnerToken: "ownerB", Name: "Production", ProjectRef: "abcd1234", Features: `["docs"]`}
	if err := s.CreateSupabaseConnection(ctx, b, enc, nonce); err != nil {
		t.Fatalf("owner B same name: %v", err)
	}

	la, err := s.ListSupabaseConnections(ctx, "ownerA")
	if err != nil || len(la) != 1 || la[0].ID != "01A" || la[0].ProjectRef != "abcd1234" {
		t.Fatalf("ownerA list = %+v err=%v", la, err)
	}

	updated, found, err := s.UpdateSupabaseConnection(ctx, "ownerA", "01A", "Staging", "efgh5678", `["database","docs","storage"]`, `["auth","storage"]`, nil, nil, false)
	if err != nil || !found || updated.Name != "Staging" || updated.ProjectRef != "efgh5678" || updated.Management != `["auth","storage"]` {
		t.Fatalf("update: found=%v err=%v row=%+v", found, err, updated)
	}
	if _, found, _ := s.UpdateSupabaseConnection(ctx, "ownerB", "01A", "Nope", "abcd1234", `["database"]`, `[]`, nil, nil, false); found {
		t.Fatal("ownerB must not update ownerA's connection")
	}

	if del, _ := s.DeleteSupabaseConnection(ctx, "ownerB", "01A"); del {
		t.Fatal("ownerB deleted ownerA's connection")
	}
	if del, _ := s.DeleteSupabaseConnection(ctx, "ownerA", "01A"); !del {
		t.Fatal("ownerA could not delete own connection")
	}
}

func TestAppSupabaseConnectionLinkClearsOnDelete(t *testing.T) {
	ctx := context.Background()
	s := newGitCredStore(t)
	conn := &SupabaseConnection{ID: "01CONN", OwnerToken: "ownerA", Name: "Production", ProjectRef: "abcd1234", Features: `["database"]`}
	if err := s.CreateSupabaseConnection(ctx, conn, []byte{1}, []byte{2}); err != nil {
		t.Fatal(err)
	}
	app := &App{ID: "01APP", OwnerToken: "ownerA", Name: "Diary"}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}
	link := "01CONN"
	if err := s.UpdateApp(ctx, app.ID, "ownerA", AppPatch{SupabaseConnectionID: &link}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAppForOwner(ctx, app.ID, "ownerA")
	if err != nil || !got.SupabaseConnectionID.Valid || got.SupabaseConnectionID.String != "01CONN" {
		t.Fatalf("link = %+v err=%v", got, err)
	}
	if _, found, err := s.GetSupabaseConnection(ctx, "ownerB", "01CONN"); err != nil || found {
		t.Fatalf("cross-owner get found=%v err=%v", found, err)
	}
	if _, err := s.DeleteSupabaseConnection(ctx, "ownerA", "01CONN"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetAppForOwner(ctx, app.ID, "ownerA")
	if err != nil || got.SupabaseConnectionID.Valid {
		t.Fatalf("link should be cleared, got %+v err=%v", got.SupabaseConnectionID, err)
	}
}
