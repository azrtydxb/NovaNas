package plugins

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

func TestToMarketplace_AllFields(t *testing.T) {
	id := uuid.New()
	addedBy := "user-1"
	now := time.Now().UTC().Truncate(time.Second)
	row := storedb.Marketplace{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		Name:        "alpha",
		IndexUrl:    "https://example/index.json",
		TrustKeyUrl: "https://example/trust.pub",
		TrustKeyPem: "PEM",
		Locked:      true,
		Enabled:     true,
		AddedBy:     &addedBy,
		AddedAt:     pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
	}
	m := toMarketplace(row)
	if m.ID != id {
		t.Errorf("id mismatch: %s vs %s", m.ID, id)
	}
	if m.Name != "alpha" || m.IndexURL != "https://example/index.json" || m.TrustKeyURL != "https://example/trust.pub" {
		t.Errorf("urls/name not propagated: %+v", m)
	}
	if m.TrustKeyPEM != "PEM" {
		t.Errorf("pem not propagated: %q", m.TrustKeyPEM)
	}
	if !m.Locked || !m.Enabled {
		t.Errorf("flags lost: %+v", m)
	}
	if m.AddedBy != "user-1" {
		t.Errorf("addedBy = %q", m.AddedBy)
	}
	if !m.AddedAt.Equal(now) || !m.UpdatedAt.Equal(now) {
		t.Errorf("timestamps lost: %+v vs %v", m, now)
	}
}

func TestToMarketplace_NullAddedBy(t *testing.T) {
	row := storedb.Marketplace{
		ID:      pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Name:    "x",
		Enabled: true,
	}
	m := toMarketplace(row)
	if m.AddedBy != "" {
		t.Errorf("addedBy expected empty, got %q", m.AddedBy)
	}
}

func TestToUUIDArg_RoundTrip(t *testing.T) {
	id := uuid.New()
	arg := toUUIDArg(id)
	if !arg.Valid {
		t.Fatal("expected valid pgtype.UUID")
	}
	got, err := uuid.FromBytes(arg.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Errorf("round-trip failed: %s vs %s", got, id)
	}
}
