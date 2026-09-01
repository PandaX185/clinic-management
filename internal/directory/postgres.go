package directory

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

type PostgresRepo struct {
	scoped *database.ScopedPool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepo) ListProfiles(ctx context.Context) ([]Profile, error) {
	var out []Profile
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListProfiles(ctx)
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]Profile, 0, len(rows))
		for _, row := range rows {
			p := Profile{
				ID:          row.ID,
				UserID:      row.UserID,
				DisplayName: row.DisplayName,
				Status:      row.Status,
				CreatedAt:   row.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   row.UpdatedAt.Format(time.RFC3339),
			}
			for _, rn := range row.RoleNames {
				p.Roles = append(p.Roles, rn)
			}
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepo) ListDoctors(ctx context.Context) ([]Profile, error) {
	var out []Profile
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListProfilesByRole(ctx, "doctor")
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]Profile, 0, len(rows))
		for _, row := range rows {
			out = append(out, Profile{
				ID:          row.ID,
				UserID:      row.UserID,
				DisplayName: row.DisplayName,
				Status:      row.Status,
				Roles:       []string{"doctor"},
				CreatedAt:   row.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   row.UpdatedAt.Format(time.RFC3339),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepo) CreateProfile(ctx context.Context, userID uuid.UUID, displayName, role string) (*Profile, error) {
	var out Profile
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		prof, err := q.CreateProfile(ctx, db.CreateProfileParams{UserID: userID, DisplayName: displayName})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					return apperr.Conflict("user already has a profile in this clinic")
				}
				if pgErr.Code == "23503" {
					return apperr.Invalid("user does not exist")
				}
			}
			return apperr.Internal(err)
		}
		rl, err := q.GetRoleByName(ctx, role)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Internal(errors.New("role not seeded in tenant schema"))
			}
			return apperr.Internal(err)
		}
		if err := q.AssignRoleToProfile(ctx, db.AssignRoleToProfileParams{ProfileID: prof.ID, RoleID: rl.ID}); err != nil {
			return apperr.Internal(err)
		}
		out = Profile{
			ID:          prof.ID,
			UserID:      prof.UserID,
			DisplayName: prof.DisplayName,
			Status:      prof.Status,
			Roles:       []string{role},
			CreatedAt:   prof.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   prof.UpdatedAt.Format(time.RFC3339),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PostgresRepo) ListAppointmentTypes(ctx context.Context) ([]AppointmentType, error) {
	var out []AppointmentType
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListAppointmentTypes(ctx)
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]AppointmentType, 0, len(rows))
		for _, row := range rows {
			out = append(out, toType(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepo) GetAppointmentType(ctx context.Context, id uuid.UUID) (*AppointmentType, error) {
	var out *AppointmentType
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetAppointmentTypeByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment type not found")
			}
			return apperr.Internal(err)
		}
		t := toType(row)
		out = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepo) CreateAppointmentType(ctx context.Context, in AppointmentType) (*AppointmentType, error) {
	price, err := decimalMoney(in.Price)
	if err != nil {
		return nil, err
	}
	var out *AppointmentType
	err = r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).CreateAppointmentType(ctx, db.CreateAppointmentTypeParams{
			Name:            in.Name,
			DurationMinutes: in.DurationMinutes,
			Price:           price,
			Color:           stringPtr(in.Color),
			Icon:            stringPtr(in.Icon),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		t := toType(row)
		out = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepo) UpdateAppointmentType(ctx context.Context, in AppointmentType) (*AppointmentType, error) {
	price, err := decimalMoney(in.Price)
	if err != nil {
		return nil, err
	}
	var out *AppointmentType
	err = r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).UpdateAppointmentType(ctx, db.UpdateAppointmentTypeParams{
			ID:              in.ID,
			Name:            in.Name,
			DurationMinutes: in.DurationMinutes,
			Price:           price,
			Color:           stringPtr(in.Color),
			Icon:            stringPtr(in.Icon),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment type not found")
			}
			return apperr.Internal(err)
		}
		t := toType(row)
		out = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// decimalMoney parses a decimal string into a pgtype.Numeric. Blank input
// (or an all-zero value) becomes zero; otherwise the string must be a valid
// decimal.
func decimalMoney(s string) (pgtype.Numeric, error) {
	s = strings.TrimSpace(s)
	var n pgtype.Numeric
	if s == "" {
		s = "0"
	}
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, apperr.Invalid("price must be a valid decimal")
	}
	return n, nil
}

func toType(row db.AppointmentType) AppointmentType {
	out := AppointmentType{
		ID:              row.ID,
		Name:            row.Name,
		DurationMinutes: row.DurationMinutes,
		Price:           numericString(row.Price),
		CreatedAt:       row.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       row.UpdatedAt.Format(time.RFC3339),
	}
	if row.Color != nil {
		out.Color = *row.Color
	}
	if row.Icon != nil {
		out.Icon = *row.Icon
	}
	return out
}

func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	v, err := n.Value()
	if err != nil {
		return "0"
	}
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return "0"
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
