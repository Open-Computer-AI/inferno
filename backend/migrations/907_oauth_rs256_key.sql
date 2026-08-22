-- The OAuth AS moved from ES256 to RS256: the gateway's own verifier
-- (plugins/dashboard_auth/nous) hard-codes algorithms=["RS256"], so the
-- constrained consumer decides. No token minted with the old key can be
-- verified any more, and none was ever issued outside test containers --
-- the AS has never been deployed. Deleting the row makes the service
-- generate a fresh RSA key on first use.
DELETE FROM security_secrets WHERE key = 'oauth_es256_active';
