package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fakeRepo is an in-memory Repository that only implements the refresh-token
// storage used by Login/Refresh/Logout. Other methods are unused by these tests.
type fakeRepo struct {
	refresh map[string]string // tokenHash -> expiry (RFC3339)
}

func (f *fakeRepo) StoreRefreshToken(_ context.Context, _ uuid.UUID, hash string, exp time.Time) error {
	f.refresh[hash] = exp.Format(time.RFC3339Nano)
	return nil
}
func (f *fakeRepo) ValidateRefreshToken(_ context.Context, _ uuid.UUID, hash string) error {
	exp, ok := f.refresh[hash]
	if !ok {
		return pgx.ErrNoRows
	}
	t, err := time.Parse(time.RFC3339Nano, exp)
	if err != nil || time.Now().After(t) {
		return pgx.ErrNoRows
	}
	return nil
}
func (f *fakeRepo) DeleteRefreshToken(_ context.Context, _ uuid.UUID, hash string) error {
	if _, ok := f.refresh[hash]; !ok {
		return pgx.ErrNoRows
	}
	delete(f.refresh, hash)
	return nil
}

// satisfy the rest of the interface with not-implemented stubs.
func (f *fakeRepo) CreateUser(context.Context, string, string, string) (*User, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRepo) GetUserByPhone(context.Context, string) (*User, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRepo) GetUserByID(_ context.Context, id uuid.UUID) (*User, error) {
	return &User{ID: id, IsActive: true, Phone: "+200000000000", FullName: "Test"}, nil
}
func (f *fakeRepo) IsGlobalAdmin(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func newTestService() (*Service, *fakeRepo) {
	repo := &fakeRepo{refresh: map[string]string{}}
	tm := NewTokenManager("access-secret", "refresh-secret", 15*time.Minute, 24*time.Hour)
	return NewService(repo, tm, 0), repo
}

func TestIssue_ParseAccessTokenRoundTrip(t *testing.T) {
	svc, _ := newTestService()
	u := &User{ID: uuid.New()}
	pair, err := svc.tokens.Issue(u)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := svc.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if claims.UserID != u.ID || claims.Type != TokenTypeAccess {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

// P0 regression: a refresh token can't be used as an access token.
func TestRefreshTokenRejectedAsAccess(t *testing.T) {
	svc, _ := newTestService()
	u := &User{ID: uuid.New()}
	pair, _ := svc.tokens.Issue(u)
	if _, err := svc.ParseAccessToken(pair.RefreshToken); err == nil {
		t.Fatal("refresh token must NOT validate as an access token")
	}
}

// P0 regression: Refresh rotates — the old token is revoked and cannot be
// used again, while the new token is valid.
func TestRefresh_RotatesAndRevokesOld(t *testing.T) {
	svc, _ := newTestService()
	u := &User{ID: uuid.New()}

	first, err := svc.tokens.Issue(u)
	if err != nil {
		t.Fatalf("issue first: %v", err)
	}
	if err := svc.repo.StoreRefreshToken(context.Background(), u.ID, HashRefreshToken(first.RefreshToken), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("store: %v", err)
	}

	second, err := svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Old token must now be invalid (revoked).
	if err := svc.repo.ValidateRefreshToken(context.Background(), u.ID, HashRefreshToken(first.RefreshToken)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("old refresh token should have been revoked after rotation")
	}
	// New token must validate.
	if err := svc.repo.ValidateRefreshToken(context.Background(), u.ID, HashRefreshToken(second.RefreshToken)); err != nil {
		t.Fatalf("new refresh token should be valid: %v", err)
	}
}

// P0 regression: Logout revokes the refresh token.
func TestLogout_RevokesToken(t *testing.T) {
	svc, _ := newTestService()
	u := &User{ID: uuid.New()}
	pair, _ := svc.tokens.Issue(u)
	if err := svc.repo.StoreRefreshToken(context.Background(), u.ID, HashRefreshToken(pair.RefreshToken), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := svc.Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := svc.repo.ValidateRefreshToken(context.Background(), u.ID, HashRefreshToken(pair.RefreshToken)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("refresh token should be revoked after logout")
	}
}

// P0 regression: Refreshing an already-revoked token fails.
func TestRefresh_RevokedTokenFails(t *testing.T) {
	svc, _ := newTestService()
	u := &User{ID: uuid.New()}
	pair, _ := svc.tokens.Issue(u)
	// Do NOT store it. Refresh must reject (not in DB).
	if _, err := svc.Refresh(context.Background(), pair.RefreshToken); err == nil {
		t.Fatal("refresh of unstored token must fail")
	}
}

// normalizePhone handles E.164 normalisation for Egyptian local numbers.
func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"+201234567890": "+201234567890",
		"01234567890":   "+201234567890",
		"201234567890":  "+201234567890",
		"1234567890":    "+1234567890",
	}
	for in, want := range cases {
		if got := normalizePhone(in); got != want {
			t.Fatalf("normalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}
