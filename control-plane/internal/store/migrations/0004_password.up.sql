-- Phase 3a — optional password credential for users (argon2id hash).
-- Enrollment stays invite-based; a password lets a user log in on a new device.
ALTER TABLE users ADD COLUMN password_hash text;
