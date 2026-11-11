package store

import (
	"context"
	"testing"
)

func TestFirebaseDistributionStoreAndAppLink(t *testing.T) {
	ctx := context.Background()
	s := newGitCredStore(t)
	enc, nonce := []byte("cipher"), []byte("nonce12")
	conn := &FirebaseDistribution{
		ID: "01FIRE", OwnerToken: "ownerA", Name: "Android", AppID: "1:123:android:abc",
		ProjectID: "coach", ClientEmail: "distro@coach.iam.gserviceaccount.com",
		GroupAlias: "testers", TesterEmails: `["ada@example.com"]`,
	}
	if err := s.CreateFirebaseDistribution(ctx, conn, enc, nonce); err != nil {
		t.Fatal(err)
	}
	dup := &FirebaseDistribution{ID: "01DUP", OwnerToken: "ownerA", Name: "Android", AppID: "1:123:android:zzz", ProjectID: "coach", ClientEmail: "a@b.c"}
	if err := s.CreateFirebaseDistribution(ctx, dup, enc, nonce); err != ErrConflict {
		t.Fatalf("dup: %v", err)
	}
	gotEnc, gotNonce, found, err := s.GetFirebaseDistributionSecret(ctx, "ownerA", "01FIRE")
	if err != nil || !found || string(gotEnc) != "cipher" || string(gotNonce) != "nonce12" {
		t.Fatalf("secret found=%v err=%v", found, err)
	}
	if _, found, err := s.GetFirebaseDistribution(ctx, "ownerB", "01FIRE"); err != nil || found {
		t.Fatalf("cross-owner found=%v err=%v", found, err)
	}

	app := &App{ID: "01APP", OwnerToken: "ownerA", Name: "Diary"}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}
	link := "01FIRE"
	if err := s.UpdateApp(ctx, app.ID, "ownerA", AppPatch{FirebaseDistributionID: &link}); err != nil {
		t.Fatal(err)
	}
	row, err := s.GetAppForOwner(ctx, app.ID, "ownerA")
	if err != nil || !row.FirebaseDistributionID.Valid || row.FirebaseDistributionID.String != "01FIRE" {
		t.Fatalf("link = %+v err=%v", row, err)
	}
	if _, err := s.DeleteFirebaseDistribution(ctx, "ownerA", "01FIRE"); err != nil {
		t.Fatal(err)
	}
	row, err = s.GetAppForOwner(ctx, app.ID, "ownerA")
	if err != nil || row.FirebaseDistributionID.Valid {
		t.Fatalf("link should be cleared, got %+v err=%v", row.FirebaseDistributionID, err)
	}
}
