package database

import "testing"

func TestValidSlug(t *testing.T) {
	ok := []string{"acme", "acme_family", "clinic_2", "a", "a0"}
	for _, s := range ok {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}
	bad := []string{"", "Acme", "acme-family", "acme!", " acme", "a_b_c_d_e_f_g_h_i_j_k_l_m_n_o_p_q_r_s_t_u_v_w_x_y_z_0_1_2_3_4_5_6",
		"1acme", "with space", "acme/../../etc"}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}

func TestSchemaNameAndRoundTrip(t *testing.T) {
	if got := SchemaName("acme_family"); got != "tenant_acme_family" {
		t.Fatalf("SchemaName = %q", got)
	}
	// Context round-trips the slug the middleware pins.
	ctx := WithTenantSlug(t.Context(), "acme_it")
	if got := TenantSlugFrom(ctx); got != "acme_it" {
		t.Fatalf("TenantSlugFrom = %q", got)
	}
	if got := TenantSlugFrom(t.Context()); got != "" {
		t.Fatalf("empty context should yield empty slug, got %q", got)
	}
}
