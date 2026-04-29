// Package plugins — pgx-backed MarketplacesStore.
//
// Thin adapter over the sqlc-generated *storedb.Queries. Kept in the
// plugins package (not internal/store) so the MarketplacesStore
// interface and value types live next to the consumer.
package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// PgxMarketplacesStore implements MarketplacesStore against the
// generated storedb.Queries. The Queries field is the only dependency.
type PgxMarketplacesStore struct {
	Q *storedb.Queries
}

// NewPgxMarketplacesStore wraps a *storedb.Queries.
func NewPgxMarketplacesStore(q *storedb.Queries) *PgxMarketplacesStore {
	return &PgxMarketplacesStore{Q: q}
}

func toMarketplace(r storedb.Marketplace) Marketplace {
	id, _ := uuid.FromBytes(r.ID.Bytes[:])
	m := Marketplace{
		ID:          id,
		Name:        r.Name,
		IndexURL:    r.IndexUrl,
		TrustKeyURL: r.TrustKeyUrl,
		TrustKeyPEM: r.TrustKeyPem,
		Locked:      r.Locked,
		Enabled:     r.Enabled,
	}
	if r.AddedBy != nil {
		m.AddedBy = *r.AddedBy
	}
	if r.AddedAt.Valid {
		m.AddedAt = r.AddedAt.Time
	}
	if r.UpdatedAt.Valid {
		m.UpdatedAt = r.UpdatedAt.Time
	}
	return m
}

func toUUIDArg(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// List returns every marketplace, locked first.
func (s *PgxMarketplacesStore) List(ctx context.Context) ([]Marketplace, error) {
	rows, err := s.Q.ListMarketplaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Marketplace, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMarketplace(r))
	}
	return out, nil
}

// ListEnabled returns marketplaces with enabled=true.
func (s *PgxMarketplacesStore) ListEnabled(ctx context.Context) ([]Marketplace, error) {
	rows, err := s.Q.ListEnabledMarketplaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Marketplace, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMarketplace(r))
	}
	return out, nil
}

// Get returns one marketplace by ID; ErrNotFound on miss.
func (s *PgxMarketplacesStore) Get(ctx context.Context, id uuid.UUID) (Marketplace, error) {
	r, err := s.Q.GetMarketplace(ctx, toUUIDArg(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Marketplace{}, ErrNotFound
		}
		return Marketplace{}, err
	}
	return toMarketplace(r), nil
}

// GetByName returns one marketplace by name.
func (s *PgxMarketplacesStore) GetByName(ctx context.Context, name string) (Marketplace, error) {
	r, err := s.Q.GetMarketplaceByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Marketplace{}, ErrNotFound
		}
		return Marketplace{}, err
	}
	return toMarketplace(r), nil
}

// Create persists a new marketplace.
func (s *PgxMarketplacesStore) Create(ctx context.Context, m Marketplace) (Marketplace, error) {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	var addedBy *string
	if m.AddedBy != "" {
		v := m.AddedBy
		addedBy = &v
	}
	r, err := s.Q.CreateMarketplace(ctx, storedb.CreateMarketplaceParams{
		ID:          toUUIDArg(m.ID),
		Name:        m.Name,
		IndexUrl:    m.IndexURL,
		TrustKeyUrl: m.TrustKeyURL,
		TrustKeyPem: m.TrustKeyPEM,
		Locked:      m.Locked,
		Enabled:     m.Enabled,
		AddedBy:     addedBy,
	})
	if err != nil {
		return Marketplace{}, fmt.Errorf("plugins: create marketplace: %w", err)
	}
	return toMarketplace(r), nil
}

// UpdateEnabled flips the enabled flag on a marketplace.
func (s *PgxMarketplacesStore) UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Marketplace, error) {
	r, err := s.Q.UpdateMarketplaceEnabled(ctx, storedb.UpdateMarketplaceEnabledParams{
		ID:      toUUIDArg(id),
		Enabled: enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Marketplace{}, ErrNotFound
		}
		return Marketplace{}, err
	}
	return toMarketplace(r), nil
}

// UpdateTrustKey replaces the pinned PEM. Audit-logging is the
// caller's responsibility (the handler emits the audit row).
func (s *PgxMarketplacesStore) UpdateTrustKey(ctx context.Context, id uuid.UUID, pem string) (Marketplace, error) {
	r, err := s.Q.UpdateMarketplaceTrustKey(ctx, storedb.UpdateMarketplaceTrustKeyParams{
		ID:          toUUIDArg(id),
		TrustKeyPem: pem,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Marketplace{}, ErrNotFound
		}
		return Marketplace{}, err
	}
	return toMarketplace(r), nil
}

// Delete removes the row. The handler refuses to call this on a
// locked entry; the DB column has no CHECK constraint, so this DAO
// happily deletes anything it is asked to.
func (s *PgxMarketplacesStore) Delete(ctx context.Context, id uuid.UUID) error {
	return s.Q.DeleteMarketplace(ctx, toUUIDArg(id))
}
