-- Per-tenant profile: binds a GLOBAL user to THIS clinic with a role.
-- Same human can be a doctor here and a patient elsewhere — role is a
-- property of the (user, clinic) pair, not of the person.
CREATE TABLE profiles (
    user_id UUID PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL REFERENCES public.roles(name) ON DELETE RESTRICT
        DEFAULT 'patient',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_profiles_role ON profiles(role) WHERE is_active;
