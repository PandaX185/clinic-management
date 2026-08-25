package database

import (
	"context"
)

type tenantSlugKeyType struct{}

var tenantSlugKey tenantSlugKeyType

// WithTenantSlug returns a context carrying the verified tenant slug. The
// slug always originates from the tenants table (never raw client input) —
// repositories use it to select the SQL schema.
func WithTenantSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, tenantSlugKey, slug)
}

// TenantSlugFrom extracts the tenant slug, failing closed with an empty
// string when absent. Callers must treat empty as "no tenant scope" and
// reject the operation rather than fall back to a default schema.
func TenantSlugFrom(ctx context.Context) string {
	slug, _ := ctx.Value(tenantSlugKey).(string)
	return slug
}
